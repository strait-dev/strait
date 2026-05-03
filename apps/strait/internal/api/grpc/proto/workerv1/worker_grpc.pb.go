// hand-written until buf codegen is wired in CI.
// TODO: replace with protoc-gen-go-grpc output once buf is installed.
package workerv1

import (
	"context"

	"google.golang.org/grpc"
)

// WorkerServiceServer is the server-side interface for the WorkerService.
type WorkerServiceServer interface {
	StreamTasks(WorkerService_StreamTasksServer) error
}

// WorkerService_StreamTasksServer is the bidirectional stream interface.
type WorkerService_StreamTasksServer interface {
	Send(*ServerMessage) error
	Recv() (*WorkerMessage, error)
	Context() context.Context
	grpc.ServerStream
}

// RegisterWorkerServiceServer registers the WorkerServiceServer implementation
// with the given gRPC server.
func RegisterWorkerServiceServer(s grpc.ServiceRegistrar, srv WorkerServiceServer) {
	s.RegisterService(&_WorkerService_serviceDesc, srv)
}

var _WorkerService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "strait.worker.v1.WorkerService",
	HandlerType: (*WorkerServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StreamTasks",
			Handler:       _WorkerService_StreamTasks_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "worker/v1/worker.proto",
}

func _WorkerService_StreamTasks_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(WorkerServiceServer).StreamTasks(&workerServiceStreamTasksServer{stream})
}

type workerServiceStreamTasksServer struct {
	grpc.ServerStream
}

func (x *workerServiceStreamTasksServer) Send(m *ServerMessage) error {
	return x.ServerStream.SendMsg(m)
}

func (x *workerServiceStreamTasksServer) Recv() (*WorkerMessage, error) {
	m := new(WorkerMessage)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}
