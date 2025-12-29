# Home Server Dashboard

A lightweight Go web dashboard for monitoring Docker Compose services and systemd services across multiple hosts.

![Dashboard Homepage with Search and Logs](docs/images/HomepageSearchLogs.png)

## Features

- Monitor Docker containers from Compose projects
- Monitor systemd units on local and remote hosts
- Real-time log streaming via Server-Sent Events
- Dark theme web interface with sorting and filtering
- Bang & Pipe query language for advanced filtering - [readme on that](docs/bangandpipe-query-language.md)
- Traefik integration for displaying exposed hostnames

## Requirements

- Go 1.21 or later
- Docker (for container monitoring)
- SSH access to remote hosts (for remote systemd monitoring)
- Traefik with API enabled (optional, for hostname discovery)

## Quick Start

1. Copy the sample configuration and edit it for your environment:

```bash
cp sample.services.json services.json
```

2. Edit `services.json` to define your hosts and services:

```json
{
  "hosts": [
    {
      "name": "myserver",
      "address": "localhost",
      "systemd_services": ["docker.service", "nginx.service"],
      "docker_compose_roots": ["/path/to/your/compose/projects/"]
    }
  ]
}
```

3. Build and run:

```bash
go build -o dashboard
./dashboard
```

4. Open http://localhost:9001 in your browser.

## How It Works

The dashboard queries Docker containers via the Docker socket and systemd units via D-Bus (for localhost) or SSH (for remote hosts). It serves a single-page web interface that fetches service status from `/api/services` and displays them in a sortable table. Clicking a service row opens an inline log viewer that streams logs in real-time using Server-Sent Events. The configuration file defines which hosts to monitor and which systemd units to track on each host. Docker Compose projects are auto-discovered by scanning the specified root directories.

## Configuration

Set `address` to `localhost` to use D-Bus for systemd queries. Any other address will use SSH with your default SSH key.

### Traefik Integration

To display Traefik-exposed hostnames as clickable links next to services, enable Traefik in your host configuration:

```json
{
  "hosts": [
    {
      "name": "myserver",
      "address": "localhost",
      "traefik": {
        "enabled": true,
        "api_port": 8080
      }
    }
  ]
}
```

The dashboard queries Traefik's `/api/http/routers` endpoint to discover which services have `Host()` rules and displays them as green hostname badges. For remote hosts, it automatically tunnels through SSH to reach the Traefik API.

## License

GPLv3
