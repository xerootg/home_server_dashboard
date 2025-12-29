# TODO

| Task | Status |
|------|--------|
| Use Bootstrap.js for all theming and for components | ✅ Done |
| Clicking on one of the counts of service status should filter to that. Clicking on the one that is selected again should unfilter | ✅ Done |
| The columns should be sortable by clicking on the table column name. Reversing order should be possible too | ✅ Done |
| Should be able to monitor systemd services on this machine and 192.168.1.9, with a filter as specified in a config file, "services.json", which is already filled out as desired. Each service should be filtered by an exact unit name match on the hostname specified. Use this tutorial as guidance https://blog.gripdev.xyz/2024/09/27/golang-restart-systemd-unit/| ✅ Done |
|Systemd should have logs. right now,  `Systemd service logs are available via journalctl: \n journalctl -u ollama.service -f is displayed`| ✅ Done |
|Break the architecture up into individual files per type of service, docker and systemd. there should be some common interface to describe the common attributes. a method to get logs, status, set status, etc. config should be shared from services.json, with a common parsing class shared between both. use go classes in this refactor (https://www.geeksforgeeks.org/go-language/class-and-object-in-golang/)| ✅ Done |
| We need tests | ✅ Done |
| break up main.go, there's unique functionality that should be isolated. seperation of concerns and all. | ✅ Done |
| Logs should have a text search for each log window that is open | ✅ Done |
| service type (systemd/docker) should be filterable | ✅ Done |
| BangAndPipe search should apply to services rendered in the table of services | ✅ Done |
| If the service exposes ports, not to localhost, but to any IP, I want a list of those after the service name, which on click opens that port as HTTP | ✅ Done |
| determine if a service is exposed in traefik. if it is, add another link to the hostname at which the service is bound | ✅ Done |
| fetch and render a description for a service from docker tags (home.server.dashboard.description) or systemd metadata (the unit file has a description field) |  |
| support getting status of services registered in traefik but not in systemd/docker |  |
| allow filtering and labeling of ports in the UI via docker labels. systemd does not expose ports, maybe later. |  |
| Write a systemd unit for this service and start it when the server starts. include a script to compile and install this binary, as well as uninstall. the config path should be configurable, and /etc/nas_dashboard/services.json should be the default path, with sample.services.json copied into that folder | ⬜ |
| all services rendered should have a start/stop/restart button. on docker, restart should down/up and not simply do a restart | ⬜ |
| a pipeline (github actions) that runs on every push which runs the tests, including integration (docker, systemd) | ⬜ |
| critical services, as defined in services.json, should be monitored for crashing. in the case of docker, if docker is not handling it via healthcheck/restart:always(or whatever), then bounce that service. systemd services should be started if stopped and there is no defined retry policy. all logs from this should be logged in stdout as well as /var/log/home_server_dashboard.log | ⬜ |
| email should be sent to contacts as defined in services.json | |
| port should be defined, optionally as well as ip, from services.json | |
| implement pub/sub for updates instead of depending on browser polling, use https://github.com/gorilla/websocket to publish updates. setup a system to monitor for state changes in go, and refactor the frontend to dynamically handle these events. | |
| Add security, using OIDC. Any user which has the admin claim can use this app. all endpoints will need to accept a token, which will need to be plumbed through to some central server-side context to verify the user is logged in |  |
| Add a security fallback, using PAM as the auth verification, as a failover when oauth is offline. issue a JWT which is valid for the PAM result. Members of the docker group have access. |  |
| run fail2ban or something infront of auth to block requests from abusive IPs |   |