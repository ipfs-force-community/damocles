# sqr-producer
This demo shows how to implment a pair of producer and consumer for some specific `Task`,
and make the producer act as a `Processor`.

You can run the compiled binary, enter numbers one line each, and see the results.

```
$ ./venus-worker/target/release/sqr-producer
2022-06-01T08:57:22.400764Z  INFO sub{name=pow pid=16170}: vc_processors::core::ext::consumer: processor ready
2022-06-01T08:57:22.400867Z  INFO parent{pid=16169}: vc_processors::core::ext::producer: producer ready
2022-06-01T08:57:22.401005Z  INFO parent{pid=16169}: sqr_producer: producer start child=16170
5
2022-06-01T08:57:23.726510Z  INFO parent{pid=16169}: sqr_producer: read in num=5
2022-06-01T08:57:23.727089Z  INFO parent{pid=16169}: sqr_producer: get output: Num(25)
9
2022-06-01T08:57:25.497819Z  INFO parent{pid=16169}: sqr_producer: read in num=9
2022-06-01T08:57:25.498182Z  INFO parent{pid=16169}: sqr_producer: get output: Num(81)
10
2022-06-01T08:57:26.738065Z  INFO parent{pid=16169}: sqr_producer: read in num=10
2022-06-01T08:57:26.738372Z  INFO parent{pid=16169}: sqr_producer: get output: Num(100)
100000
2022-06-01T08:57:29.243007Z  INFO parent{pid=16169}: sqr_producer: read in num=100000
2022-06-01T08:57:29.243334Z  WARN parent{pid=16169}: sqr_producer: get err: too large!!
```
