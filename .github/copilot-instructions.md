# Pilot Finance - Project Instructions

## Environment
- This project runs on **Docker** (not Kubernetes).

## Development Rules
- **Ports**: Never expose ports in `docker-compose.yml` or Dockerfiles.
- **Optimization**: After initial successful startup, everything that can be optimized must be.
- **Security**: Indicate whenever permission changes (chmod/chown) are required on the host.
- **Styling**: Always maintain the current theme, logo and alignment.