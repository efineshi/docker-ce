// +build !windows

package libcontainerd // import "github.com/docker/docker/libcontainerd"

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	apievents "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/archive"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/content"
	containerderrors "github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/events"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/runtime/linux/runctypes"
	"github.com/containerd/typeurl"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/ioutils"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// InitProcessName is the name given to the first process of a
// container
const InitProcessName = "init"

type container struct {
	mu sync.Mutex

	bundleDir string
	ctr       containerd.Container
	task      containerd.Task
	execs     map[string]containerd.Process
	oomKilled bool
}

func (c *container) setTask(t containerd.Task) {
	c.mu.Lock()
	c.task = t
	c.mu.Unlock()
}

func (c *container) getTask() containerd.Task {
	c.mu.Lock()
	t := c.task
	c.mu.Unlock()
	return t
}

func (c *container) addProcess(id string, p containerd.Process) {
	c.mu.Lock()
	if c.execs == nil {
		c.execs = make(map[string]containerd.Process)
	}
	c.execs[id] = p
	c.mu.Unlock()
}

func (c *container) deleteProcess(id string) {
	c.mu.Lock()
	delete(c.execs, id)
	c.mu.Unlock()
}

func (c *container) getProcess(id string) containerd.Process {
	c.mu.Lock()
	p := c.execs[id]
	c.mu.Unlock()
	return p
}

func (c *container) setOOMKilled(killed bool) {
	c.mu.Lock()
	c.oomKilled = killed
	c.mu.Unlock()
}

func (c *container) getOOMKilled() bool {
	c.mu.Lock()
	killed := c.oomKilled
	c.mu.Unlock()
	return killed
}

type client struct {
	sync.RWMutex // protects containers map

	client   *containerd.Client
	stateDir string
	logger   *logrus.Entry
	ns       string

	backend    Backend
	eventQ     queue
	containers map[string]*container
}

// NewClient creates a new libcontainerd client from a containerd client
func NewClient(ctx context.Context, cli *containerd.Client, stateDir, ns string, b Backend) (Client, error) {
	c := &client{
		client:     cli,
		stateDir:   stateDir,
		logger:     logrus.WithField("module", "libcontainerd").WithField("namespace", ns),
		ns:         ns,
		backend:    b,
		containers: make(map[string]*container),
	}

	go c.processEventStream(ctx, ns)

	return c, nil
}

func (c *client) Version(ctx context.Context) (containerd.Version, error) {
	return c.client.Version(ctx)
}

// Restore loads the containerd container.
// It should not be called concurrently with any other operation for the given ID.
func (c *client) Restore(ctx context.Context, id string, attachStdio StdioCallback) (alive bool, pid int, err error) {
	c.Lock()
	_, ok := c.containers[id]
	if ok {
		c.Unlock()
		return false, 0, errors.WithStack(newConflictError("id already in use"))
	}

	cntr := &container{}
	c.containers[id] = cntr
	cntr.mu.Lock()
	defer cntr.mu.Unlock()

	c.Unlock()

	defer func() {
		if err != nil {
			c.Lock()
			delete(c.containers, id)
			c.Unlock()
		}
	}()

	var dio *cio.DirectIO
	defer func() {
		if err != nil && dio != nil {
			dio.Cancel()
			dio.Close()
		}
		err = wrapError(err)
	}()

	ctr, err := c.client.LoadContainer(ctx, id)
	if err != nil {
		return false, -1, errors.WithStack(wrapError(err))
	}

	attachIO := func(fifos *cio.FIFOSet) (cio.IO, error) {
		// dio must be assigned to the previously defined dio for the defer above
		// to handle cleanup
		dio, err = cio.NewDirectIO(ctx, fifos)
		if err != nil {
			return nil, err
		}
		return attachStdio(dio)
	}
	t, err := ctr.Task(ctx, attachIO)
	if err != nil && !containerderrors.IsNotFound(err) {
		return false, -1, errors.Wrap(wrapError(err), "error getting containerd task for container")
	}

	if t != nil {
		s, err := t.Status(ctx)
		if err != nil {
			return false, -1, errors.Wrap(wrapError(err), "error getting task status")
		}

		alive = s.Status != containerd.Stopped
		pid = int(t.Pid())
	}

	cntr.bundleDir = filepath.Join(c.stateDir, id)
	cntr.ctr = ctr
	cntr.task = t
	// TODO(mlaventure): load execs

	c.logger.WithFields(logrus.Fields{
		"container": id,
		"alive":     alive,
		"pid":       pid,
	}).Debug("restored container")

	return alive, pid, nil
}

func (c *client) Create(ctx context.Context, id string, ociSpec *specs.Spec, runtimeOptions interface{}) error {
	if ctr := c.getContainer(id); ctr != nil {
		return errors.WithStack(newConflictError("id already in use"))
	}

	bdir, err := prepareBundleDir(filepath.Join(c.stateDir, id), ociSpec)
	if err != nil {
		return errdefs.System(errors.Wrap(err, "prepare bundle dir failed"))
	}

	c.logger.WithField("bundle", bdir).WithField("root", ociSpec.Root.Path).Debug("bundle dir created")

	cdCtr, err := c.client.NewContainer(ctx, id,
		containerd.WithSpec(ociSpec),
		// TODO(mlaventure): when containerd support lcow, revisit runtime value
		containerd.WithRuntime(fmt.Sprintf("io.containerd.runtime.v1.%s", runtime.GOOS), runtimeOptions))
	if err != nil {
		return wrapError(err)
	}

	c.Lock()
	c.containers[id] = &container{
		bundleDir: bdir,
		ctr:       cdCtr,
	}
	c.Unlock()

	return nil
}

// Start create and start a task for the specified containerd id
func (c *client) Start(ctx context.Context, id, checkpointDir string, withStdin bool, attachStdio StdioCallback) (int, error) {
	ctr := c.getContainer(id)
	if ctr == nil {
		return -1, errors.WithStack(newNotFoundError("no such container"))
	}
	if t := ctr.getTask(); t != nil {
		return -1, errors.WithStack(newConflictError("container already started"))
	}

	var (
		cp             *types.Descriptor
		t              containerd.Task
		rio            cio.IO
		err            error
		stdinCloseSync = make(chan struct{})
	)

	if checkpointDir != "" {
		// write checkpoint to the content store
		tar := archive.Diff(ctx, "", checkpointDir)
		cp, err = c.writeContent(ctx, images.MediaTypeContainerd1Checkpoint, checkpointDir, tar)
		// remove the checkpoint when we're done
		defer func() {
			if cp != nil {
				err := c.client.ContentStore().Delete(context.Background(), cp.Digest)
				if err != nil {
					c.logger.WithError(err).WithFields(logrus.Fields{
						"ref":    checkpointDir,
						"digest": cp.Digest,
					}).Warnf("failed to delete temporary checkpoint entry")
				}
			}
		}()
		if err := tar.Close(); err != nil {
			return -1, errors.Wrap(err, "failed to close checkpoint tar stream")
		}
		if err != nil {
			return -1, errors.Wrapf(err, "failed to upload checkpoint to containerd")
		}
	}

	spec, err := ctr.ctr.Spec(ctx)
	if err != nil {
		return -1, errors.Wrap(err, "failed to retrieve spec")
	}
	uid, gid := getSpecUser(spec)
	t, err = ctr.ctr.NewTask(ctx,
		func(id string) (cio.IO, error) {
			fifos := newFIFOSet(ctr.bundleDir, InitProcessName, withStdin, spec.Process.Terminal)

			rio, err = c.createIO(fifos, id, InitProcessName, stdinCloseSync, attachStdio)
			return rio, err
		},
		func(_ context.Context, _ *containerd.Client, info *containerd.TaskInfo) error {
			info.Checkpoint = cp
			info.Options = &runctypes.CreateOptions{
				IoUid:       uint32(uid),
				IoGid:       uint32(gid),
				NoPivotRoot: os.Getenv("DOCKER_RAMDISK") != "",
			}
			return nil
		})
	if err != nil {
		close(stdinCloseSync)
		if rio != nil {
			rio.Cancel()
			rio.Close()
		}
		return -1, wrapError(err)
	}

	ctr.setTask(t)

	// Signal c.createIO that it can call CloseIO
	close(stdinCloseSync)

	if err := t.Start(ctx); err != nil {
		if _, err := t.Delete(ctx); err != nil {
			c.logger.WithError(err).WithField("container", id).
				Error("failed to delete task after fail start")
		}
		ctr.setTask(nil)
		return -1, wrapError(err)
	}

	return int(t.Pid()), nil
}

// Exec creates exec process.
//
// The containerd client calls Exec to register the exec config in the shim side.
// When the client calls Start, the shim will create stdin fifo if needs. But
// for the container main process, the stdin fifo will be created in Create not
// the Start call. stdinCloseSync channel should be closed after Start exec
// process.
func (c *client) Exec(ctx context.Context, containerID, processID string, spec *specs.Process, withStdin bool, attachStdio StdioCallback) (int, error) {
	ctr := c.getContainer(containerID)
	if ctr == nil {
		return -1, errors.WithStack(newNotFoundError("no such container"))
	}
	t := ctr.getTask()
	if t == nil {
		return -1, errors.WithStack(newInvalidParameterError("container is not running"))
	}

	if p := ctr.getProcess(processID); p != nil {
		return -1, errors.WithStack(newConflictError("id already in use"))
	}

	var (
		p              containerd.Process
		rio            cio.IO
		err            error
		stdinCloseSync = make(chan struct{})
	)

	fifos := newFIFOSet(ctr.bundleDir, processID, withStdin, spec.Terminal)

	defer func() {
		if err != nil {
			if rio != nil {
				rio.Cancel()
				rio.Close()
			}
		}
	}()

	p, err = t.Exec(ctx, processID, spec, func(id string) (cio.IO, error) {
		rio, err = c.createIO(fifos, containerID, processID, stdinCloseSync, attachStdio)
		return rio, err
	})
	if err != nil {
		close(stdinCloseSync)
		return -1, wrapError(err)
	}

	ctr.addProcess(processID, p)

	// Signal c.createIO that it can call CloseIO
	//
	// the stdin of exec process will be created after p.Start in containerd
	defer close(stdinCloseSync)

	if err = p.Start(ctx); err != nil {
		// use new context for cleanup because old one may be cancelled by user, but leave a timeout to make sure
		// we are not waiting forever if containerd is unresponsive or to work around fifo cancelling issues in
		// older containerd-shim
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		p.Delete(ctx)
		ctr.deleteProcess(processID)
		return -1, wrapError(err)
	}

	return int(p.Pid()), nil
}

func (c *client) SignalProcess(ctx context.Context, containerID, processID string, signal int) error {
	p, err := c.getProcess(containerID, processID)
	if err != nil {
		return err
	}
	return wrapError(p.Kill(ctx, syscall.Signal(signal)))
}

func (c *client) ResizeTerminal(ctx context.Context, containerID, processID string, width, height int) error {
	p, err := c.getProcess(containerID, processID)
	if err != nil {
		return err
	}

	return p.Resize(ctx, uint32(width), uint32(height))
}

func (c *client) CloseStdin(ctx context.Context, containerID, processID string) error {
	p, err := c.getProcess(containerID, processID)
	if err != nil {
		return err
	}

	return p.CloseIO(ctx, containerd.WithStdinCloser)
}

func (c *client) Pause(ctx context.Context, containerID string) error {
	p, err := c.getProcess(containerID, InitProcessName)
	if err != nil {
		return err
	}

	return wrapError(p.(containerd.Task).Pause(ctx))
}

func (c *client) Resume(ctx context.Context, containerID string) error {
	p, err := c.getProcess(containerID, InitProcessName)
	if err != nil {
		return err
	}

	return p.(containerd.Task).Resume(ctx)
}

func (c *client) Stats(ctx context.Context, containerID string) (*Stats, error) {
	p, err := c.getProcess(containerID, InitProcessName)
	if err != nil {
		return nil, err
	}

	m, err := p.(containerd.Task).Metrics(ctx)
	if err != nil {
		return nil, err
	}

	v, err := typeurl.UnmarshalAny(m.Data)
	if err != nil {
		return nil, err
	}
	return interfaceToStats(m.Timestamp, v), nil
}

func (c *client) ListPids(ctx context.Context, containerID string) ([]uint32, error) {
	p, err := c.getProcess(containerID, InitProcessName)
	if err != nil {
		return nil, err
	}

	pis, err := p.(containerd.Task).Pids(ctx)
	if err != nil {
		return nil, err
	}

	var pids []uint32
	for _, i := range pis {
		pids = append(pids, i.Pid)
	}

	return pids, nil
}

func (c *client) Summary(ctx context.Context, containerID string) ([]Summary, error) {
	p, err := c.getProcess(containerID, InitProcessName)
	if err != nil {
		return nil, err
	}

	pis, err := p.(containerd.Task).Pids(ctx)
	if err != nil {
		return nil, err
	}

	var infos []Summary
	for _, pi := range pis {
		i, err := typeurl.UnmarshalAny(pi.Info)
		if err != nil {
			return nil, errors.Wrap(err, "unable to decode process details")
		}
		s, err := summaryFromInterface(i)
		if err != nil {
			return nil, err
		}
		infos = append(infos, *s)
	}

	return infos, nil
}

func (c *client) DeleteTask(ctx context.Context, containerID string) (uint32, time.Time, error) {
	p, err := c.getProcess(containerID, InitProcessName)
	if err != nil {
		return 255, time.Now(), nil
	}

	status, err := p.(containerd.Task).Delete(ctx)
	if err != nil {
		return 255, time.Now(), nil
	}

	if ctr := c.getContainer(containerID); ctr != nil {
		ctr.setTask(nil)
	}
	return status.ExitCode(), status.ExitTime(), nil
}

func (c *client) Delete(ctx context.Context, containerID string) error {
	ctr := c.getContainer(containerID)
	if ctr == nil {
		return errors.WithStack(newNotFoundError("no such container"))
	}

	if err := ctr.ctr.Delete(ctx); err != nil {
		return wrapError(err)
	}

	if os.Getenv("LIBCONTAINERD_NOCLEAN") != "1" {
		if err := os.RemoveAll(ctr.bundleDir); err != nil {
			c.logger.WithError(err).WithFields(logrus.Fields{
				"container": containerID,
				"bundle":    ctr.bundleDir,
			}).Error("failed to remove state dir")
		}
	}

	c.removeContainer(containerID)

	return nil
}

func (c *client) Status(ctx context.Context, containerID string) (Status, error) {
	ctr := c.getContainer(containerID)
	if ctr == nil {
		return StatusUnknown, errors.WithStack(newNotFoundError("no such container"))
	}

	t := ctr.getTask()
	if t == nil {
		return StatusUnknown, errors.WithStack(newNotFoundError("no such task"))
	}

	s, err := t.Status(ctx)
	if err != nil {
		return StatusUnknown, wrapError(err)
	}

	return Status(s.Status), nil
}

func (c *client) CreateCheckpoint(ctx context.Context, containerID, checkpointDir string, exit bool) error {
	p, err := c.getProcess(containerID, InitProcessName)
	if err != nil {
		return err
	}

	opts := []containerd.CheckpointTaskOpts{}
	if exit {
		opts = append(opts, func(r *containerd.CheckpointTaskInfo) error {
			if r.Options == nil {
				r.Options = &runctypes.CheckpointOptions{
					Exit: true,
				}
			} else {
				opts, _ := r.Options.(*runctypes.CheckpointOptions)
				opts.Exit = true
			}
			return nil
		})
	}
	img, err := p.(containerd.Task).Checkpoint(ctx, opts...)
	if err != nil {
		return wrapError(err)
	}
	// Whatever happens, delete the checkpoint from containerd
	defer func() {
		err := c.client.ImageService().Delete(context.Background(), img.Name())
		if err != nil {
			c.logger.WithError(err).WithField("digest", img.Target().Digest).
				Warnf("failed to delete checkpoint image")
		}
	}()

	b, err := content.ReadBlob(ctx, c.client.ContentStore(), img.Target())
	if err != nil {
		return errdefs.System(errors.Wrapf(err, "failed to retrieve checkpoint data"))
	}
	var index v1.Index
	if err := json.Unmarshal(b, &index); err != nil {
		return errdefs.System(errors.Wrapf(err, "failed to decode checkpoint data"))
	}

	var cpDesc *v1.Descriptor
	for _, m := range index.Manifests {
		if m.MediaType == images.MediaTypeContainerd1Checkpoint {
			cpDesc = &m
			break
		}
	}
	if cpDesc == nil {
		return errdefs.System(errors.Wrapf(err, "invalid checkpoint"))
	}

	rat, err := c.client.ContentStore().ReaderAt(ctx, *cpDesc)
	if err != nil {
		return errdefs.System(errors.Wrapf(err, "failed to get checkpoint reader"))
	}
	defer rat.Close()
	_, err = archive.Apply(ctx, checkpointDir, content.NewReader(rat))
	if err != nil {
		return errdefs.System(errors.Wrapf(err, "failed to read checkpoint reader"))
	}

	return err
}

func (c *client) getContainer(id string) *container {
	c.RLock()
	ctr := c.containers[id]
	c.RUnlock()

	return ctr
}

func (c *client) removeContainer(id string) {
	c.Lock()
	delete(c.containers, id)
	c.Unlock()
}

func (c *client) getProcess(containerID, processID string) (containerd.Process, error) {
	ctr := c.getContainer(containerID)
	if ctr == nil {
		return nil, errors.WithStack(newNotFoundError("no such container"))
	}

	t := ctr.getTask()
	if t == nil {
		return nil, errors.WithStack(newNotFoundError("container is not running"))
	}
	if processID == InitProcessName {
		return t, nil
	}

	p := ctr.getProcess(processID)
	if p == nil {
		return nil, errors.WithStack(newNotFoundError("no such exec"))
	}
	return p, nil
}

// createIO creates the io to be used by a process
// This needs to get a pointer to interface as upon closure the process may not have yet been registered
func (c *client) createIO(fifos *cio.FIFOSet, containerID, processID string, stdinCloseSync chan struct{}, attachStdio StdioCallback) (cio.IO, error) {
	var (
		io  *cio.DirectIO
		err error
	)

	io, err = cio.NewDirectIO(context.Background(), fifos)
	if err != nil {
		return nil, err
	}

	if io.Stdin != nil {
		var (
			err       error
			stdinOnce sync.Once
		)
		pipe := io.Stdin
		io.Stdin = ioutils.NewWriteCloserWrapper(pipe, func() error {
			stdinOnce.Do(func() {
				err = pipe.Close()
				// Do the rest in a new routine to avoid a deadlock if the
				// Exec/Start call failed.
				go func() {
					<-stdinCloseSync
					p, err := c.getProcess(containerID, processID)
					if err == nil {
						err = p.CloseIO(context.Background(), containerd.WithStdinCloser)
						if err != nil && strings.Contains(err.Error(), "transport is closing") {
							err = nil
						}
					}
				}()
			})
			return err
		})
	}

	rio, err := attachStdio(io)
	if err != nil {
		io.Cancel()
		io.Close()
	}
	return rio, err
}

func (c *client) processEvent(ctr *container, et EventType, ei EventInfo) {
	c.eventQ.append(ei.ContainerID, func() {
		err := c.backend.ProcessEvent(ei.ContainerID, et, ei)
		if err != nil {
			c.logger.WithError(err).WithFields(logrus.Fields{
				"container":  ei.ContainerID,
				"event":      et,
				"event-info": ei,
			}).Error("failed to process event")
		}

		if et == EventExit && ei.ProcessID != ei.ContainerID {
			p := ctr.getProcess(ei.ProcessID)
			if p == nil {
				c.logger.WithError(errors.New("no such process")).
					WithFields(logrus.Fields{
						"container": ei.ContainerID,
						"process":   ei.ProcessID,
					}).Error("exit event")
				return
			}
			_, err = p.Delete(context.Background())
			if err != nil {
				c.logger.WithError(err).WithFields(logrus.Fields{
					"container": ei.ContainerID,
					"process":   ei.ProcessID,
				}).Warn("failed to delete process")
			}
			ctr.deleteProcess(ei.ProcessID)

			ctr := c.getContainer(ei.ContainerID)
			if ctr == nil {
				c.logger.WithFields(logrus.Fields{
					"container": ei.ContainerID,
				}).Error("failed to find container")
			} else {
				newFIFOSet(ctr.bundleDir, ei.ProcessID, true, false).Close()
			}
		}
	})
}

func (c *client) processEventStream(ctx context.Context, ns string) {
	var (
		err error
		ev  *events.Envelope
		et  EventType
		ei  EventInfo
		ctr *container
	)

	// Filter on both namespace *and* topic. To create an "and" filter,
	// this must be a single, comma-separated string
	eventStream, errC := c.client.EventService().Subscribe(ctx, "namespace=="+ns+",topic~=|^/tasks/|")

	c.logger.Debug("processing event stream")

	var oomKilled bool
	for {
		select {
		case err = <-errC:
			if err != nil {
				errStatus, ok := status.FromError(err)
				if !ok || errStatus.Code() != codes.Canceled {
					c.logger.WithError(err).Error("failed to get event")

					// rate limit
					select {
					case <-time.After(time.Second):
						go c.processEventStream(ctx, ns)
						return
					case <-ctx.Done():
					}
				}
				c.logger.WithError(ctx.Err()).Info("stopping event stream following graceful shutdown")
			}
			return
		case ev = <-eventStream:
			if ev.Event == nil {
				c.logger.WithField("event", ev).Warn("invalid event")
				continue
			}

			v, err := typeurl.UnmarshalAny(ev.Event)
			if err != nil {
				c.logger.WithError(err).WithField("event", ev).Warn("failed to unmarshal event")
				continue
			}

			c.logger.WithField("topic", ev.Topic).Debug("event")

			switch t := v.(type) {
			case *apievents.TaskCreate:
				et = EventCreate
				ei = EventInfo{
					ContainerID: t.ContainerID,
					ProcessID:   t.ContainerID,
					Pid:         t.Pid,
				}
			case *apievents.TaskStart:
				et = EventStart
				ei = EventInfo{
					ContainerID: t.ContainerID,
					ProcessID:   t.ContainerID,
					Pid:         t.Pid,
				}
			case *apievents.TaskExit:
				et = EventExit
				ei = EventInfo{
					ContainerID: t.ContainerID,
					ProcessID:   t.ID,
					Pid:         t.Pid,
					ExitCode:    t.ExitStatus,
					ExitedAt:    t.ExitedAt,
				}
			case *apievents.TaskOOM:
				et = EventOOM
				ei = EventInfo{
					ContainerID: t.ContainerID,
					OOMKilled:   true,
				}
				oomKilled = true
			case *apievents.TaskExecAdded:
				et = EventExecAdded
				ei = EventInfo{
					ContainerID: t.ContainerID,
					ProcessID:   t.ExecID,
				}
			case *apievents.TaskExecStarted:
				et = EventExecStarted
				ei = EventInfo{
					ContainerID: t.ContainerID,
					ProcessID:   t.ExecID,
					Pid:         t.Pid,
				}
			case *apievents.TaskPaused:
				et = EventPaused
				ei = EventInfo{
					ContainerID: t.ContainerID,
				}
			case *apievents.TaskResumed:
				et = EventResumed
				ei = EventInfo{
					ContainerID: t.ContainerID,
				}
			default:
				c.logger.WithFields(logrus.Fields{
					"topic": ev.Topic,
					"type":  reflect.TypeOf(t)},
				).Info("ignoring event")
				continue
			}

			ctr = c.getContainer(ei.ContainerID)
			if ctr == nil {
				c.logger.WithField("container", ei.ContainerID).Warn("unknown container")
				continue
			}

			if oomKilled {
				ctr.setOOMKilled(true)
				oomKilled = false
			}
			ei.OOMKilled = ctr.getOOMKilled()

			c.processEvent(ctr, et, ei)
		}
	}
}

func (c *client) writeContent(ctx context.Context, mediaType, ref string, r io.Reader) (*types.Descriptor, error) {
	writer, err := c.client.ContentStore().Writer(ctx, content.WithRef(ref))
	if err != nil {
		return nil, err
	}
	defer writer.Close()
	size, err := io.Copy(writer, r)
	if err != nil {
		return nil, err
	}
	labels := map[string]string{
		"containerd.io/gc.root": time.Now().UTC().Format(time.RFC3339),
	}
	if err := writer.Commit(ctx, 0, "", content.WithLabels(labels)); err != nil {
		return nil, err
	}
	return &types.Descriptor{
		MediaType: mediaType,
		Digest:    writer.Digest(),
		Size_:     size,
	}, nil
}

func wrapError(err error) error {
	switch {
	case err == nil:
		return nil
	case containerderrors.IsNotFound(err):
		return errdefs.NotFound(err)
	}

	msg := err.Error()
	for _, s := range []string{"container does not exist", "not found", "no such container"} {
		if strings.Contains(msg, s) {
			return errdefs.NotFound(err)
		}
	}
	return err
}
