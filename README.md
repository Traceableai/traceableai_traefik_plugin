# Configuration

To configure the Traceable Traefik plugin you will need a Traceable Platform Agent deployed and configured.
In Kubernetes, you could deploy TME as a sidecar to your traefik deployment.

In your `traefik.yml` you need to add the plugin configuration:

```yaml
experimental:
  plugins:
    plugin-traceableai:
      moduleName: github.com/traceableai/traceable_traefik_plugin
      moduleVersion: 1.0.0
```

Then add the plugin as a middleware & configure it on any router you want to collect data for:
```yaml
http:
  routers:
    whoami-router:
      rule: "Host(`echo-service.docker.localhost`)"
      service: echo-service
      middlewares:
        - my-plugin-traceableai
  
  
  middlewares:
    my-plugin-traceableai:
      plugin:
        plugin-traceableai:
          allowedContentTypes:
            - "json"
            - "xml"
            - "x-www-form-urlencoded"
            - "grpc"
          bodyCaptureSize: 131072
          serviceName: traefik
          tpaEndpoint: http://REPLACE_ME_WITH_TPA_HOST_OR_IP:5442
```

## Local Testing
To test traefik locally, first navigate to the `local` directory.

Run `docker-compose up reverse-proxy` to start traefik.

In another terminal, run `docker-compose up echo-service` to start a echo server.

You can access the traefik dashboard at `http://localhost:8080`.

You can access the echo service by sendina  `Host` header set to `echo-service.docker.localhost`.
Example: `curl -H "Host: echo-service.docker.localhost" http://localhost:80`
