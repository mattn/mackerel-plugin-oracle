mackerel-plugin-oracle
======================

Oracle custom metrics plugin for mackerel.io agent.

## Synopsis

```shell
mackerel-plugin-oracle -dsn=<DSN> -event=<event> -event<event> ...
```

`-dsn` is database source name.
`-event` is Oracle WaitEvent name.

## Example of mackerel-agent.conf

```
[plugin.metrics.oracle]
command = [
	"/path/to/mackerel-plugin-oracle",
	"-event=Disk File Operations I/O",
	"-event=control file sequential read",
	"-event=OS Thread Startup",
	"-dsn=scott/tiger@XE"
]
```

mackerel-agent is executed by root user. And root user probably doesn't have Oracle setup like environment variables. If your OS use yum/rpm, add following like to load `oracle_env.sh` into `/etc/sysconfig/mackerel-agent`.

```
. /u01/app/oracle/product/11.2.0/xe/bin/oracle_env.sh
```
This is an example of the case using Oracle XE 11.2.

If your OS use apt/deb, the path should be `/etc/default/mackerel-agent`.

<https://mackerel.io/ja/docs/entry/spec/agent>

## Reference

You can find event name 

```
SELECT name, wait_class FROM V$EVENT_NAME ORDER BY name;
```

See also: <https://docs.oracle.com/database/122/REFRN/descriptions-of-wait-events.htm#REFRN-GUID-2FDDFAA4-24D0-4B80-A157-A907AF5C68E2>
