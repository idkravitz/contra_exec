FROM golang:latest

MAINTAINER Dmitry Kravtsov <idkravitz@gmail.com>

RUN apt-get update && apt-get -y install dos2unix unzip p7zip-full git build-essential cmake
RUN apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# http proxy support
RUN [ -n "$http_proxy" ] && apt-get update && apt-get install -y corkscrew && \
	echo "ProxyCommand corkscrew $http_proxy %h %p" | sed 's/:/ /g' >> /etc/ssh/config || echo 'No proxy'

RUN cd /usr/local && mkdir transims4 transims4/bin && cd transims4 && \
	git clone https://github.com/kravitz/transims4.git src && mkdir build && \
	cd build && cmake ../src && make -j2 && cp bin/* ../bin && cd .. && \
	rm -rf src build

RUN useradd -r -m tram
USER tram
WORKDIR /home/tram

RUN mkdir -p bin src/github.com/kravitz/tram_exec pkg
ENV GOPATH /home/tram
ENV PATH /usr/local/transims4/bin:$PATH
RUN go get gopkg.in/mgo.v2 && go get github.com/streadway/amqp

ADD . src/github.com/kravitz/tram_exec

RUN go install github.com/kravitz/tram_exec
EXPOSE 8080

ENTRYPOINT ["./bin/tram_exec"]
