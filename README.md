# Zoidberg TCP

This is TCP balancer managed by [Zoidberg](https://github.com/bobrik/zoidberg).

## Features

* Zero configuration, only options to set are management host and port.
* Marathon ready, no wrappers needed to run on Marathon.
* Connection retries in case that upstream server does not respond.

## Usage

### Running proxy

This runs `zoidberg-tcp` with management interface on `http://127.0.0.1:12345`:

```
docker run -rm -it --net host \
  -e HOST=127.0.0.1 -e PORT=12345 \
  bobrik/zoidberg-tcp:0.1.0
```

It's up to you how to discover launched balancer in
[Zoidberg](https://github.com/bobrik/zoidberg). Both static (list of servers)
and dynamic (`mesos` or `marathon` finders) are supported.

### Making apps available

For `mesos` and `marathon` finders the following labels should be set
to make app available:

* `zoidberg_app_name` application name, doesn't really matter now.
* `zoidberg_balanced_by` load balancer name to announce.
* `zoidberg_meta_port_X_type` set port at index `X` to the mode `tcp`.
* `zoidberg_meta_port_X_listen` set listen (`host:port`) for port at index X.

Example:

```yaml
zoidberg_app_name: myapp.example.com
zoidberg_balanced_by: example-lb-tcp
zoidberg_meta_port_0_type: tcp
zoidberg_meta_port_0_listen: 127.0.0.1:23232
```

Application with these labels will be announced on all load balancers with name
set to `example-lb-tcp`. On these load balancers `127.0.0.1:23232` will be
forwarding connections to all application instances on the port at index `0`.

It is possible to use [zoidberg-nginx](https://github.com/bobrik/zoidberg-nginx)
with `zoidberg-tcp` when some ports are HTTP and some ports are plain TCP.

## TODO

* [SO_REUSEPORT](https://lwn.net/Articles/542629/)
* Weighted least connections balancing mode
