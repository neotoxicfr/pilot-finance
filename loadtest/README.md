# Load Tests (k6)

## Install k6

```bash
# macOS
brew install k6

# Linux (Debian/Ubuntu)
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D68
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update && sudo apt-get install k6

# Docker
docker run --rm -i grafana/k6 run - <smoke.js
```

## Run

```bash
# Smoke test (5 VUs, 30s)
k6 run loadtest/smoke.js

# Stress test (ramp to 50 VUs)
k6 run loadtest/stress.js

# Custom base URL
k6 run -e BASE_URL=https://pilot.example.com loadtest/smoke.js
```

## Thresholds

| Test   | p95 latency | p99 latency | Error rate |
|--------|-------------|-------------|------------|
| Smoke  | < 500ms     | < 1s        | < 1%       |
| Stress | < 1s        | < 2s        | < 5%       |
