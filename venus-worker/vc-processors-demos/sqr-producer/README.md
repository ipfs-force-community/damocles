# sqr-producer
This demo shows how to implment a pair of producer and consumer for some specific `Task`,
and make the producer act as a `Processor`.

You can run the compiled binary, enter numbers one line each, and see the results.

```
$ ./target/release/sqr-producer
2022-06-01T09:57:46.873986Z  INFO sub{name=pow pid=21576}: vc_processors::core::ext::consumer: processor ready
2022-06-01T09:57:46.874102Z  INFO parent{pid=21575}: vc_processors::core::ext::producer: producer ready
2022-06-01T09:57:46.874238Z  INFO parent{pid=21575}: sqr_producer: producer start child=21576
2022-06-01T09:57:46.874327Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
5
2022-06-01T09:57:48.304364Z  INFO parent{pid=21575}: sqr_producer: read in num=5
2022-06-01T09:57:48.304923Z  INFO parent{pid=21575}: sqr_producer: get output: Num(25)
2022-06-01T09:57:48.304967Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
7
2022-06-01T09:57:49.654865Z  INFO parent{pid=21575}: sqr_producer: read in num=7
2022-06-01T09:57:49.655199Z  INFO parent{pid=21575}: sqr_producer: get output: Num(49)
2022-06-01T09:57:49.655230Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
10
2022-06-01T09:57:51.044910Z  INFO parent{pid=21575}: sqr_producer: read in num=10
2022-06-01T09:57:51.045184Z  INFO parent{pid=21575}: sqr_producer: get output: Num(100)
2022-06-01T09:57:51.045212Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
100000
2022-06-01T09:57:53.320905Z  INFO parent{pid=21575}: sqr_producer: read in num=100000
2022-06-01T09:57:53.321201Z  WARN parent{pid=21575}: sqr_producer: get err: too large!!
2022-06-01T09:57:53.321234Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
```
