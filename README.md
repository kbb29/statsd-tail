statsd-tail
===========

Listens for statsd (in Datadog's dialect) and pretty-prints it on the console.

    > statsd-tail
    foo.bar  map[key:value]  1
    foo.bar  map[key:value]  2
    ...

Getting it
----------

Provided a working Go-setup, install the code with

```go install github.com/kbb29/statsd-tail@latest```

run with

```go run github.com/kbb29/statsd-tail -h```

or

```~/go/bin/statsd-tail -h```



you can specify the --host, --port to listen on
you can specify the --interval at which you want metrics to be aggregated and displayed
(set to 0 to dump the metrics as they arrive)

License
-------

MIT - Do what you want!
