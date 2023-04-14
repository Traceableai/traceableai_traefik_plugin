# Run this from this directory
# make sure you have grpcurl installed

grpcurl -vv -H="Host: frontend.local" -d '{"name": "Alice"}' \
--plaintext -import-path=$(pwd) \
-proto=helloworld.proto 127.0.0.1:80 helloworld.Greeter/SayHello