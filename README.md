# PikPakDAV

PikPakDAV is a simple WebDAV server for [PikPak](https://mypikpak.com). The intended use case is to use it in conjunction with [rclone](https://rclone.org) to download files from PikPak.

## Installation

Use docker images at [Docker Hub](https://hub.docker.com/r/gyf304/pikpakdav).

## Usage

```bash
docker run -d -p 8080:8080 gyf304/pikpakdav
```

Use `http://localhost:8080` as WebDAV server address. Use your PikPak username and password to login.

## Supported Operations

The server is a read-only WebDAV server, with additional DELETE support.
