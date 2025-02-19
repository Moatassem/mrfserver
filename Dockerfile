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