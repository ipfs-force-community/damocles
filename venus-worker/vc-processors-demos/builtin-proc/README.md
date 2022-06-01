# builtin-proc
This demo shows how to use builtin tasks & processors.

Also, it shows how to set a simple rate limiter with hooks.

```
$ ./target/release/builtin-proc
2022-06-01T09:53:30.693717Z  INFO sub{name=tree_d pid=21252}: vc_processors::core::ext::consumer: processor ready
2022-06-01T09:53:30.693839Z  INFO parent{pid=21250}: vc_processors::core::ext::producer: producer ready
2022-06-01T09:53:30.693959Z  INFO parent{pid=21250}: builtin_proc: producer start child=21252
2022-06-01T09:53:30.694103Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
a
2022-06-01T09:53:31.248063Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="a"
2022-06-01T09:53:31.249032Z  INFO parent{pid=21250}: builtin_proc: token acquired
2022-06-01T09:53:31.255737Z  INFO parent{pid=21250}: builtin_proc: do nothing
2022-06-01T09:53:31.255784Z  INFO parent{pid=21250}: builtin_proc: get output: true
2022-06-01T09:53:31.256044Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
b
2022-06-01T09:53:31.994187Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="b"
2022-06-01T09:53:35.686187Z  INFO builtin_proc: re-fill one token
2022-06-01T09:53:35.686225Z  INFO parent{pid=21250}: builtin_proc: token acquired
2022-06-01T09:53:35.688114Z  INFO parent{pid=21250}: builtin_proc: do nothing
2022-06-01T09:53:35.688158Z  INFO parent{pid=21250}: builtin_proc: get output: true
2022-06-01T09:53:35.688397Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
c
2022-06-01T09:53:36.468706Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="c"
2022-06-01T09:53:40.686604Z  INFO builtin_proc: re-fill one token
2022-06-01T09:53:40.686655Z  INFO parent{pid=21250}: builtin_proc: token acquired
2022-06-01T09:53:40.687885Z  INFO parent{pid=21250}: builtin_proc: do nothing
2022-06-01T09:53:40.687928Z  INFO parent{pid=21250}: builtin_proc: get output: true
2022-06-01T09:53:40.688241Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
d
2022-06-01T09:53:45.481275Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="d"
2022-06-01T09:53:45.686773Z  INFO builtin_proc: re-fill one token
2022-06-01T09:53:45.686818Z  INFO parent{pid=21250}: builtin_proc: token acquired
2022-06-01T09:53:45.688524Z  INFO parent{pid=21250}: builtin_proc: do nothing
2022-06-01T09:53:45.688568Z  INFO parent{pid=21250}: builtin_proc: get output: true
2022-06-01T09:53:45.688884Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
e2022-06-01T09:53:50.687008Z  INFO builtin_proc: re-fill one token

2022-06-01T09:53:53.877925Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="e"
2022-06-01T09:53:53.878905Z  INFO parent{pid=21250}: builtin_proc: token acquired
2022-06-01T09:53:53.880119Z  INFO parent{pid=21250}: builtin_proc: do nothing
2022-06-01T09:53:53.880161Z  INFO parent{pid=21250}: builtin_proc: get output: true
2022-06-01T09:53:53.880457Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
2022-06-01T09:53:55.687241Z  INFO builtin_proc: re-fill one token
```
