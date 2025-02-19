FROM golang:alpine AS build
LABEL maintainer="eng.moatassem@gmail.com"

WORKDIR /mrfgo

COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

COPY . .
RUN go build -o mrfgo .

FROM alpine AS run
LABEL maintainer="eng.moatassem@gmail.com"

RUN mkdir /mrfgo

COPY --from=build /mrfgo/mrfgo /mrfgo/mrfgo

WORKDIR /mrfgo

CMD ["./mrfgo"]




# check README.md for more information on how to build and run the docker image
# docker build -t mrfgo:latest .
# docker run -d --name mrfgo -p 5060:5060/udp -p 8080:8080 -e as_sip_udp="#.#.#.#:####" -e server_ipv4="#.#.#.#" -e sip_udp_port="5060" -e http_port="8080" mrfgo:latest
# docker run -d --name mrfgo --net=host -e as_sip_udp="#.#.#.#:####" -e server_ipv4="#.#.#.#" -e sip_udp_port="5060" -e http_port="8080" mrfgo:latest


# Replace #.#.#.#:#### with the IP:Port of Kasuar or NewkahGoSIP
# Replace #.#.#.# with the IP of SR own IP used in SIP and HTTP