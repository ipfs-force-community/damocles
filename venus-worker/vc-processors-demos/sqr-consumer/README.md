# sqr-consumer
This demo shows how to implment an ext consumer for some specific `Task`.

You can start the compiled binary, enter something like `{"id": 1, "task": 15}` to the input,
and then see the result from stdout.

```
2022-06-01T07:41:34.114535Z  INFO sqr_consumer: start sqr consumer
pow processor ready
2022-06-01T07:41:34.114678Z  INFO sub{name=pow pid=10951}: vc_processors::core::ext::consumer: processor ready
{"id": 1, "task": 15}
{"id":1,"err_msg":null,"output":225}
```

You can use any number you like to replace `15`, and see what happens.
