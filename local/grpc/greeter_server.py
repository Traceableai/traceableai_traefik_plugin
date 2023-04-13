# # # Copyright 2018 The gRPC Authors
# # #
# # # Licensed under the Apache License, Version 2.0 (the "License");
# # # you may not use this file except in compliance with the License.
# # # You may obtain a copy of the License at
# # #
# # #     http://www.apache.org/licenses/LICENSE-2.0
# # #
# # # Unless required by applicable law or agreed to in writing, software
# # # distributed under the License is distributed on an "AS IS" BASIS,
# # # WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# # # See the License for the specific language governing permissions and
# # # limitations under the License.
# # """The reflection-enabled version of gRPC helloworld.Greeter server."""
# #
# # from concurrent import futures
# # import logging
# #
# # import grpc
# # from grpc_reflection.v1alpha import reflection
# # import helloworld_pb2
# # import helloworld_pb2_grpc
# #
# #
# # class Greeter(helloworld_pb2_grpc.GreeterServicer):
# #
# #     def SayHello(self, request, context):
# #         return helloworld_pb2.HelloReply(message='Hello, %s!' % request.name)
# #
# #
# # def serve():
# #     server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
# #
# #
# #     # Add the Greeter service to the server
# #     helloworld_pb2_grpc.add_GreeterServicer_to_server(Greeter(), server)
# #
# #     # Enable server reflection
# #     SERVICE_NAMES = (
# #         helloworld_pb2.DESCRIPTOR.services_by_name['Greeter'].full_name,
# #         reflection.SERVICE_NAME,
# #     )
# #     reflection.enable_server_reflection(SERVICE_NAMES, server)
# #
# #     print("About to start the server!")
# #     # Start the server
# #     server.start()
# #     server.wait_for_termination()
# #
# #
# # if __name__ == '__main__':
# #     logging.basicConfig()
# #     serve()
#
# from concurrent import futures
# import logging
#
# import grpc
# import helloworld_pb2
# import helloworld_pb2_grpc
#
#
# class Greeter(helloworld_pb2_grpc.GreeterServicer):
#
#     def SayHello(self, request, context):
#         return helloworld_pb2.HelloReply(message='Hello, %s!' % request.name)
#
#
# def serve():
#     # Load the self-signed certificate and private key
#     with open('backend.crt', 'rb') as f:
#         server_cert = f.read()
#     with open('backend.key', 'rb') as f:
#         server_key = f.read()
#
#     # Create an SSL context and configure it with the certificate and private key
#     server_credentials = grpc.ssl_server_credentials([(server_key, server_cert)])
#     server = grpc.server(futures.ThreadPoolExecutor(max_workers=10), options=[('grpc.ssl_target_name_override', 'frontend.local')])
#     server.add_secure_port('0.0.0.0:10443', server_credentials)
#     helloworld_pb2_grpc.add_GreeterServicer_to_server(Greeter(), server)
#
#     server.start()
#     logging.error("Server started on port 10443")
#     server.wait_for_termination()
#
#
# if __name__ == '__main__':
#     logging.basicConfig()
#     serve()
from concurrent import futures
import logging

import grpc
import helloworld_pb2
import helloworld_pb2_grpc


class Greeter(helloworld_pb2_grpc.GreeterServicer):

    def SayHello(self, request, context):
        return helloworld_pb2.HelloReply(message='Hello, %s!' % request.name)


def serve():
    port = '8090'
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    helloworld_pb2_grpc.add_GreeterServicer_to_server(Greeter(), server)
    server.add_insecure_port('[::]:' + port)
    server.start()
    print("Server started, listening on " + port)
    server.wait_for_termination()


if __name__ == '__main__':
    logging.basicConfig()
    serve()