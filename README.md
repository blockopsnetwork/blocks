<p align="center">
  <a href="https://app.blockops.network" title="Blockops Network">
    <img src="./assets/img/blockops-logo.png" alt="Blockops-Network-logo" width="244" />
  </a>
</p>

<h1 align="center">BlocksOp Network</h1>

- [Summary](#summary)
- [Language](#language)
- [License](#license)


## Summary

An All-in-One Web3 Observability tooling that collects metrics and logs from blockchain nodes and related infrastructure.

## Usage

Telescope can be configured either through command line flags or a YAML configuration file.

Using Command Line Flags
Basic usage with metrics enabled:

```bash
agent \
  --metrics \
  --network=ethereum \
  --project-id=my-project \
  --project-name=my-name \
  --telescope-username=user \
  --telescope-password=pass \
  --remote-write-url=https://metrics.example.com
```

Enable both metrics and logs:

```bash
agent \
  --metrics \
  --enable-logs \
  --network=ethereum \
  --project-id=my-project \
  --project-name=my-name \
  --telescope-username=user \
  --telescope-password=pass \
  --remote-write-url=https://metrics.example.com \
  --logs-sink-url=https://logs.example.com \
  --telescope-loki-username=loki-user \
  --telescope-loki-password=loki-pass
```

#### Using Configuration File

Create a YAML configuration file and run:

```bash
agent --config-file=telescope_config.yaml
```

Example configuration file:

```yaml
server:
  log_level: info
metrics:
  global:
    scrape_interval: 15s
    external_labels:
      project_id: my-project
      project_name: my-name
    remote_write:
      - url: https://metrics.example.com
        basic_auth:
          username: user
          password: pass
  wal_directory: /tmp/telescope
  configs:
    - name: my-name_ethereum_metrics
      host_filter: false
      scrape_configs:
        - job_name: ethereum
          static_configs:
            - targets: ["localhost:8545"]
logs:
  configs:
    - name: telescope_logs
      clients:
        - url: https://logs.example.com
          basic_auth:
            username: loki-user
            password: loki-pass
          external_labels:
            project_id: my-project
            project_name: my-name
      positions:
        filename: /tmp/telescope_logs
integrations:
  agent:
    enabled: false
  node_exporter:
    enabled: true
```



## Language
- Golang

## Contributing
We would love to work with anyone who can contribute their work and improve this project. The details will be shared soon.


## License

Licensed Under [Apache 2.0](./LICENSE)