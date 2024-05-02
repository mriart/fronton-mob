FROM ubuntu:latest


COPY . ./

EXPOSE 8080
CMD ["./server/server"]
