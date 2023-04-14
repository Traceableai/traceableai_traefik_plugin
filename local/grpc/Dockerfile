FROM python:3.9.6

RUN python -m pip install grpcio-tools
RUN git clone -b v1.53.0 --depth 1 --shallow-submodules https://github.com/grpc/grpc
RUN rm grpc/examples/python/helloworld/greeter_server.py

COPY greeter_server.py grpc/examples/python/helloworld/

ENV GRPC_VERBOSITY=debug
EXPOSE 8080
CMD ["python", "grpc/examples/python/helloworld/greeter_server.py"]