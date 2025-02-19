# Media Resource Function (MRF) Server v1.0 - written from scratch in Golang

- Highly optimized, high performance, modular, carrier-grade MRFGo with capacity **exceeding 500 CAPS** and **50,000 concurrent sessions**
- Highly customizable MRFGo supporting Media Server Control
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

Environment variables must be defined in order to launch MRFGo container.

-e server_ipv4="#.#.#.#:####"

-e media_dir="..." path for the directory holding the raw PCM files

-e sip_udp_port="5060" (optional)

-e http_port="8080" (optional)

## Notes

Use SoX: https://en.wikipedia.org/wiki/SoX

- For Windows: https://sourceforge.net/projects/sox/
- For Linux/Ubuntu: https://manpages.ubuntu.com/manpages/focal/man1/sox.1.html
- Syntax: "sox --clobber --no-glob "<audiofile>" -e signed-integer -b 16 -c 1 -r 16000 "<audiofile>.raw" speed 2
- Throw in the generates raw files inside the Media Directory and MRFGo will read them during startup

## Author

- **Moatassem TALAAT** - _Complete implementation_ -
