ARG GO_IMAGE
ARG ENGINE_IMAGE
ARG BUILD_IMAGE=centos:7
FROM ${GO_IMAGE} as golang
FROM ${ENGINE_IMAGE} as engine

FROM ${BUILD_IMAGE}
ENV DISTRO centos
ENV SUITE 7
ENV GOPATH=/go
ENV PATH $PATH:/usr/local/go/bin:$GOPATH/bin
ENV AUTO_GOPATH 1
ENV DOCKER_BUILDTAGS seccomp selinux
ENV RUNC_BUILDTAGS seccomp selinux
RUN yum install -y rpm-build rpmlint
COPY SPECS /root/rpmbuild/SPECS
# Overwrite repo that was failing on aarch64
RUN sed -i 's/altarch/centos/g' /etc/yum.repos.d/CentOS-Sources.repo
RUN yum-builddep -y /root/rpmbuild/SPECS/*.spec
COPY --from=golang /usr/local/go /usr/local/go/
COPY --from=engine /bin/dockerd /sources/
COPY --from=engine /bin/docker-proxy /sources/
COPY --from=engine /bin/docker-init /sources/
WORKDIR /root/rpmbuild
ENTRYPOINT ["/bin/rpmbuild"]
