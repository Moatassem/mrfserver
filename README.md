# Media Resource Function (MRF) Server v1.0 - written from scratch in Golang

- Highly optimized, high performance, modular, carrier-grade MRF with capacity **exceeding 1000 CAPS** and **50,000 concurrent sessions**
- Highly customizable MRF supporting Media Server Control
- Docker-containerized compiled with golang:alpine

## Launching the build

Construction is automatic via Git CI/CD pipeline

## Service Details

Notes below ports are exposed **by default**:

- UDP 5060 for SIP
- TCP 8080 for HTTP Web API + Prometheus integration

## Routing Logic

- MRFGo has pools of directory number and associated audio files
- MRFGo supports PCMA, PCMU, G722 ... soon G729 and OPUS

## Environment Variables

Environment variables must be defined in order to launch SR container.

-e server_ipv4="#.#.#.#:####"

-e media_path="..." path for the directory holding the raw PCM files "sox --clobber --no-glob "dir" -e signed-integer -b 16 -c 1 -r 16000 "\*.raw" speed 2

-e sip_udp_port="5060" (optional)

-e http_port="8080" (optional)

## Author

- **Moatassem TALAAT** - _Complete implementation_ -
