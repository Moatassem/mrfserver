FROM golang:alpine AS build
LABEL maintainer="eng.moatassem@gmail.com"

WORKDIR /mrfgo

COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

# Copy everything except ./audio
COPY . .
RUN rm -rf ./audio
RUN go build -o mrfgo .

FROM alpine AS run
LABEL maintainer="eng.moatassem@gmail.com"

RUN mkdir -p /mrfgo/audio

COPY --from=build /mrfgo/mrfgo /mrfgo/mrfgo
COPY ./audio /mrfgo/audio

WORKDIR /mrfgo

CMD ["./mrfgo"]