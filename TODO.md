# TODO

| Task | Status |
|------|--------|
| Use Bootstrap.js for all theming and for components | ✅ Done |
| Clicking on one of the counts of service status should filter to that. Clicking on the one that is selected again should unfilter | ✅ Done |
| The columns should be sortable by clicking on the table column name. Reversing order should be possible too | ✅ Done |
| Should be able to monitor systemd services on this machine and 192.168.1.9, with a filter as specified in a config file, "services.json", which is already filled out as desired. Each service should be filtered by an exact unit name match on the hostname specified. Use this tutorial as guidance https://blog.gripdev.xyz/2024/09/27/golang-restart-systemd-unit/| ✅ Done |
|Systemd should have logs. right now,  `Systemd service logs are available via journalctl: \n journalctl -u ollama.service -f is displayed`| ✅ Done |
|Break the architecture up into individual files per type of service, docker and systemd. there should be some common interface to describe the common attributes. a method to get logs, status, set status, etc. config should be shared from services.json, with a common parsing class shared between both. use go classes in this refactor (https://www.geeksforgeeks.org/go-language/class-and-object-in-golang/)| ✅ Done |
| We need tests | ⬜ |
| Logs should have a text search | ⬜ |
| If the service exposes ports, not to localhost, but to any IP, I want a list of those after the service name, which on click opens that port as HTTP | ⬜ |
| Write a systemd unit for this service and start it when the server starts | ⬜ |
| all services rendered should have a start/stop/restart button | ⬜ |