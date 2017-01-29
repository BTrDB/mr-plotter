FROM ubuntu:xenial

# we install etcdctl so that you can exec etcd commands inside the container
ENV ETCDCTL_VERSION v3.1.0
ENV ETCDCTL_ARCH linux-amd64

RUN apt-get update && apt-get install -y vim wget net-tools git build-essential && \
    apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* && \
    wget -q -O- "https://github.com/coreos/etcd/releases/download/${ETCDCTL_VERSION}/etcd-${ETCDCTL_VERSION}-${ETCDCTL_ARCH}.tar.gz" |tar xfz - -C/tmp/ etcd-${ETCDCTL_VERSION}-${ETCDCTL_ARCH}/etcdctl && \
    mv /tmp/etcd-${ETCDCTL_VERSION}-${ETCDCTL_ARCH}/etcdctl /usr/local/bin/etcdctl
RUN wget -O /tmp/go.tar.gz https://storage.googleapis.com/golang/go1.7.4.linux-amd64.tar.gz && tar -xf /tmp/go.tar.gz -C /usr/local/ && rm /tmp/go.tar.gz && mkdir /srv/go
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin:/srv/target:/srv/go/bin GOPATH=/srv/go
ENV GOTRACEBACK=all
ENV GOGC=40
RUN go get github.com/maruel/panicparse/cmd/pp
RUN mkdir -p $GOPATH/src/github.com/SoftwareDefinedBuildings && \
    cd $GOPATH/src/github.com/SoftwareDefinedBuildings && \
    git clone https://github.com/SoftwareDefinedBuildings/mr-plotter.git && \
    cd mr-plotter && \
    git checkout v4 && \
    go get -d ./... && \
    go install .
ADD entrypoint.sh /
RUN chmod a+x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
