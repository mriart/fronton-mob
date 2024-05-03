FROM ubuntu:latest


COPY . /

WORKDIR /server
EXPOSE 8080
CMD ["/server/server"]
