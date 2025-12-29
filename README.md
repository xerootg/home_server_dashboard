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

### Service Descriptions

The dashboard displays descriptions for services when available:

- **Docker**: Add a `home.server.dashboard.description` label to your container in `docker-compose.yml`:
  ```yaml
  services:
    myapp:
      labels:
        - "home.server.dashboard.description=My application description"
  ```

- **Systemd**: Descriptions are automatically fetched from the unit's `Description` field.

## Service Control Setup

The dashboard can start, stop, and restart services. This requires proper authentication setup.

### SSH Key Authentication (Remote Hosts)

For remote systemd hosts, SSH key-based authentication must be configured:

```bash
# Generate an SSH key if you don't have one
ssh-keygen -t ed25519 -C "dashboard"

# Copy the key to remote hosts
ssh-copy-id user@192.168.1.9
```

### Sudoers Configuration

Systemctl commands require root privileges. Configure passwordless sudo for only the specific services you want to manage.

**Generate sudoers configuration automatically from your `services.json`:**

```bash
# Generate for current user
./dashboard -generate-sudoers

# Generate for a specific user
./dashboard -generate-sudoers -sudoers-user myuser
```

This outputs a sudoers configuration based on your configured systemd services. Install it with:

```bash
./dashboard -generate-sudoers | sudo tee /etc/sudoers.d/home-server-dashboard
sudo chmod 440 /etc/sudoers.d/home-server-dashboard
```

For remote hosts, copy the relevant lines to each remote machine's `/etc/sudoers.d/home-server-dashboard`.

**Manual configuration** (if preferred):

On each host (local and remote), create a sudoers file:

```bash
sudo visudo -f /etc/sudoers.d/home-server-dashboard
```

Add rules for the specific services (replace `youruser` and service names):

```sudoers
# Allow dashboard user to manage specific systemd services without password
youruser ALL=(ALL) NOPASSWD: /usr/bin/systemctl start ollama.service
youruser ALL=(ALL) NOPASSWD: /usr/bin/systemctl stop ollama.service
youruser ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart ollama.service
youruser ALL=(ALL) NOPASSWD: /usr/bin/systemctl start docker.service
youruser ALL=(ALL) NOPASSWD: /usr/bin/systemctl stop docker.service
youruser ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart docker.service
```

**Or use a pattern to allow all systemctl operations on specific services:**

```sudoers
# Allow start/stop/restart for specific services
youruser ALL=(ALL) NOPASSWD: /usr/bin/systemctl start ollama.service, \
                             /usr/bin/systemctl stop ollama.service, \
                             /usr/bin/systemctl restart ollama.service
```

**For Docker containers**, the user running the dashboard needs to be in the `docker` group:

```bash
sudo usermod -aG docker youruser
# Log out and back in for group changes to take effect
```

### Security Considerations

- **Principle of Least Privilege**: Only grant sudo access to the specific services listed in your `services.json`
- **Avoid wildcards**: Don't use `systemctl *` patterns in sudoers
- **SSH hardening**: Consider using a dedicated SSH key for the dashboard and restricting it with `ForceCommand` if needed

## License

GPLv3
