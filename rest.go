/*
 *
 * Copyright 2020 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/asjard/genproto/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	contextPackage = protogen.GoImportPath("context")
	restPackage    = protogen.GoImportPath("github.com/asjard/asjard/pkg/server/rest")
	serverPackage  = protogen.GoImportPath("github.com/asjard/asjard/core/server")
	// restPackage    = protogen.GoImportPath("google.golang.org/grpc")
	// codesPackage   = protogen.GoImportPath("google.golang.org/grpc/codes")
	// statusPackage  = protogen.GoImportPath("google.golang.org/grpc/status")
)

type serviceGenerateHelperInterface interface {
	formatFullMethodSymbol(service *protogen.Service, method *protogen.Method) string
	genFullMethods(g *protogen.GeneratedFile, service *protogen.Service)
	generateClientStruct(g *protogen.GeneratedFile, clientName string)
	generateNewClientDefinitions(g *protogen.GeneratedFile, service *protogen.Service, clientName string)
	generateUnimplementedServerType(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile, service *protogen.Service)
	generateServerFunctions(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile, service *protogen.Service, serverType string, serviceDescVar string)
	formatHandlerFuncName(service *protogen.Service, hname string) string
}

type serviceGenerateHelper struct{}

func (serviceGenerateHelper) formatFullMethodSymbol(service *protogen.Service, method *protogen.Method) string {
	return fmt.Sprintf("%s_%s_FullMethodName", service.GoName, method.GoName)
}

func (serviceGenerateHelper) genFullMethods(g *protogen.GeneratedFile, service *protogen.Service) {
	if len(service.Methods) == 0 {
		return
	}
	g.P()
}

func (serviceGenerateHelper) generateClientStruct(g *protogen.GeneratedFile, clientName string) {
	g.P("type ", unexport(clientName), " struct {")
	// g.P("cc ", restPackage.Ident("ClientConnInterface"))
	g.P("}")
	g.P()
}

func (serviceGenerateHelper) generateNewClientDefinitions(g *protogen.GeneratedFile, service *protogen.Service, clientName string) {
	g.P("return &", unexport(clientName), "{cc}")
}

func (serviceGenerateHelper) generateUnimplementedServerType(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile, service *protogen.Service) {
}

func (serviceGenerateHelper) generateServerFunctions(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile, service *protogen.Service, serverType string, serviceDescVar string) {
	// Server handler implementations.
	handlerNames := make([]string, 0, len(service.Methods))
	for _, method := range service.Methods {
		hname := genServerMethod(gen, file, g, method, serverType, func(hname string) string {
			return hname
		})
		handlerNames = append(handlerNames, hname)
	}
	genServiceDesc(file, g, serviceDescVar, serverType, service, handlerNames)
}

func (serviceGenerateHelper) formatHandlerFuncName(service *protogen.Service, hname string) string {
	return hname
}

var helper serviceGenerateHelperInterface = serviceGenerateHelper{}

// FileDescriptorProto.package field number
const fileDescriptorProtoPackageFieldNumber = 2

// FileDescriptorProto.syntax field number
const fileDescriptorProtoSyntaxFieldNumber = 12

// generateFile generates a _grpc.pb.go file containing gRPC service definitions.
func generateFile(gen *protogen.Plugin, file *protogen.File) *protogen.GeneratedFile {
	if len(file.Services) == 0 {
		return nil
	}
	filename := file.GeneratedFilenamePrefix + "_rest.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	// Attach all comments associated with the syntax field.
	genLeadingComments(g, file.Desc.SourceLocations().ByPath(protoreflect.SourcePath{fileDescriptorProtoSyntaxFieldNumber}))
	g.P("// Code generated by protoc-gen-go-grpc. DO NOT EDIT.")
	g.P("// versions:")
	g.P("// - protoc-gen-go-rest v", version)
	g.P("// - protoc             ", protocVersion(gen))
	if file.Proto.GetOptions().GetDeprecated() {
		g.P("// ", file.Desc.Path(), " is a deprecated file.")
	} else {
		g.P("// source: ", file.Desc.Path())
	}
	g.P()
	// Attach all comments associated with the package field.
	genLeadingComments(g, file.Desc.SourceLocations().ByPath(protoreflect.SourcePath{fileDescriptorProtoPackageFieldNumber}))
	g.P("package ", file.GoPackageName)
	g.P()
	generateFileContent(gen, file, g)
	return g
}

func protocVersion(gen *protogen.Plugin) string {
	v := gen.Request.GetCompilerVersion()
	if v == nil {
		return "(unknown)"
	}
	var suffix string
	if s := v.GetSuffix(); s != "" {
		suffix = "-" + s
	}
	return fmt.Sprintf("v%d.%d.%d%s", v.GetMajor(), v.GetMinor(), v.GetPatch(), suffix)
}

// generateFileContent generates the gRPC service definitions, excluding the package statement.
func generateFileContent(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile) {
	if len(file.Services) == 0 {
		return
	}
	g.P()
	for _, service := range file.Services {
		genService(gen, file, g, service)
	}
}

// genServiceComments copies the comments from the RPC proto definitions
// to the corresponding generated interface file.
func genServiceComments(g *protogen.GeneratedFile, service *protogen.Service) {
	if service.Comments.Leading != "" {
		// Add empty comment line to attach this service's comments to
		// the godoc comments previously output for all services.
		g.P("//")
		g.P(strings.TrimSpace(service.Comments.Leading.String()))
	}
}

func genService(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile, service *protogen.Service) {
	// Full methods constants.
	helper.genFullMethods(g, service)

	serverType := service.GoName + "Server"
	serviceDescVar := service.GoName + "RestServiceDesc"
	helper.generateServerFunctions(gen, file, g, service, serverType, serviceDescVar)
}

func clientSignature(g *protogen.GeneratedFile, method *protogen.Method) string {
	s := method.GoName + "(ctx " + g.QualifiedGoIdent(contextPackage.Ident("Context"))
	if !method.Desc.IsStreamingClient() {
		s += ", in *" + g.QualifiedGoIdent(method.Input.GoIdent)
	}
	s += ", opts ..." + g.QualifiedGoIdent(restPackage.Ident("CallOption")) + ") ("
	if !method.Desc.IsStreamingClient() && !method.Desc.IsStreamingServer() {
		s += "*" + g.QualifiedGoIdent(method.Output.GoIdent)
	} else {
		if *useGenericStreams {
			s += clientStreamInterface(g, method)
		} else {
			s += method.Parent.GoName + "_" + method.GoName + "Client"
		}
	}
	s += ", error)"
	return s
}

func clientStreamInterface(g *protogen.GeneratedFile, method *protogen.Method) string {
	typeParam := g.QualifiedGoIdent(method.Input.GoIdent) + ", " + g.QualifiedGoIdent(method.Output.GoIdent)
	if method.Desc.IsStreamingClient() && method.Desc.IsStreamingServer() {
		return g.QualifiedGoIdent(restPackage.Ident("BidiStreamingClient")) + "[" + typeParam + "]"
	} else if method.Desc.IsStreamingClient() {
		return g.QualifiedGoIdent(restPackage.Ident("ClientStreamingClient")) + "[" + typeParam + "]"
	} else { // i.e. if method.Desc.IsStreamingServer()
		return g.QualifiedGoIdent(restPackage.Ident("ServerStreamingClient")) + "[" + g.QualifiedGoIdent(method.Output.GoIdent) + "]"
	}
}

func genClientMethod(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile, method *protogen.Method, index int) {
	service := method.Parent
	fmSymbol := helper.formatFullMethodSymbol(service, method)

	if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
		g.P(deprecationComment)
	}
	g.P("func (c *", unexport(service.GoName), "Client) ", clientSignature(g, method), "{")
	g.P("cOpts := append([]", restPackage.Ident("CallOption"), "{", restPackage.Ident("StaticMethod()"), "}, opts...)")
	if !method.Desc.IsStreamingServer() && !method.Desc.IsStreamingClient() {
		g.P("out := new(", method.Output.GoIdent, ")")
		g.P(`err := c.cc.Invoke(ctx, `, fmSymbol, `, in, out, cOpts...)`)
		g.P("if err != nil { return nil, err }")
		g.P("return out, nil")
		g.P("}")
		g.P()
		return
	}

	streamImpl := unexport(service.GoName) + method.GoName + "Client"
	if *useGenericStreams {
		typeParam := g.QualifiedGoIdent(method.Input.GoIdent) + ", " + g.QualifiedGoIdent(method.Output.GoIdent)
		streamImpl = g.QualifiedGoIdent(restPackage.Ident("GenericClientStream")) + "[" + typeParam + "]"
	}

	serviceDescVar := service.GoName + "_ServiceDesc"
	g.P("stream, err := c.cc.NewStream(ctx, &", serviceDescVar, ".Streams[", index, `], `, fmSymbol, `, cOpts...)`)
	g.P("if err != nil { return nil, err }")
	g.P("x := &", streamImpl, "{ClientStream: stream}")
	if !method.Desc.IsStreamingClient() {
		g.P("if err := x.ClientStream.SendMsg(in); err != nil { return nil, err }")
		g.P("if err := x.ClientStream.CloseSend(); err != nil { return nil, err }")
	}
	g.P("return x, nil")
	g.P("}")
	g.P()

	// Auxiliary types aliases, for backwards compatibility.
	if *useGenericStreams {
		g.P("// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.")
		g.P("type ", service.GoName, "_", method.GoName, "Client = ", clientStreamInterface(g, method))
		g.P()
		return
	}

	// Stream auxiliary types and methods, if we're not taking advantage of the
	// pre-implemented generic types and their methods.
	genSend := method.Desc.IsStreamingClient()
	genRecv := method.Desc.IsStreamingServer()
	genCloseAndRecv := !method.Desc.IsStreamingServer()

	g.P("type ", service.GoName, "_", method.GoName, "Client interface {")
	if genSend {
		g.P("Send(*", method.Input.GoIdent, ") error")
	}
	if genRecv {
		g.P("Recv() (*", method.Output.GoIdent, ", error)")
	}
	if genCloseAndRecv {
		g.P("CloseAndRecv() (*", method.Output.GoIdent, ", error)")
	}
	g.P(restPackage.Ident("ClientStream"))
	g.P("}")
	g.P()

	g.P("type ", streamImpl, " struct {")
	g.P(restPackage.Ident("ClientStream"))
	g.P("}")
	g.P()

	if genSend {
		g.P("func (x *", streamImpl, ") Send(m *", method.Input.GoIdent, ") error {")
		g.P("return x.ClientStream.SendMsg(m)")
		g.P("}")
		g.P()
	}
	if genRecv {
		g.P("func (x *", streamImpl, ") Recv() (*", method.Output.GoIdent, ", error) {")
		g.P("m := new(", method.Output.GoIdent, ")")
		g.P("if err := x.ClientStream.RecvMsg(m); err != nil { return nil, err }")
		g.P("return m, nil")
		g.P("}")
		g.P()
	}
	if genCloseAndRecv {
		g.P("func (x *", streamImpl, ") CloseAndRecv() (*", method.Output.GoIdent, ", error) {")
		g.P("if err := x.ClientStream.CloseSend(); err != nil { return nil, err }")
		g.P("m := new(", method.Output.GoIdent, ")")
		g.P("if err := x.ClientStream.RecvMsg(m); err != nil { return nil, err }")
		g.P("return m, nil")
		g.P("}")
		g.P()
	}
}

func serverSignature(g *protogen.GeneratedFile, method *protogen.Method) string {
	var reqArgs []string
	ret := "error"
	if !method.Desc.IsStreamingClient() && !method.Desc.IsStreamingServer() {
		reqArgs = append(reqArgs, g.QualifiedGoIdent(contextPackage.Ident("Context")))
		ret = "(*" + g.QualifiedGoIdent(method.Output.GoIdent) + ", error)"
	}
	if !method.Desc.IsStreamingClient() {
		reqArgs = append(reqArgs, "*"+g.QualifiedGoIdent(method.Input.GoIdent))
	}
	if method.Desc.IsStreamingClient() || method.Desc.IsStreamingServer() {
		if *useGenericStreams {
			reqArgs = append(reqArgs, serverStreamInterface(g, method))
		} else {
			reqArgs = append(reqArgs, method.Parent.GoName+"_"+method.GoName+"Server")
		}
	}
	return method.GoName + "(" + strings.Join(reqArgs, ", ") + ") " + ret
}

func genServiceDesc(file *protogen.File, g *protogen.GeneratedFile, serviceDescVar string, serverType string, service *protogen.Service, handlerNames []string) {
	// Service descriptor.
	g.P("// ", serviceDescVar, " is the ", restPackage.Ident("ServiceDesc"), " for ", service.GoName, " service.")
	g.P("// It's only intended for direct use with ", restPackage.Ident("AddHandler"), ",")
	g.P("// and not to be introspected or modified (even as a copy)")
	g.P("var ", serviceDescVar, " = ", restPackage.Ident("ServiceDesc"), " {")
	g.P("ServiceName: ", strconv.Quote(string(service.Desc.FullName())), ",")
	g.P("HandlerType: (*", serverType, ")(nil),")
	g.P("Methods: []", restPackage.Ident("MethodDesc"), "{")
	for i, method := range service.Methods {
		if method.Desc.IsStreamingClient() || method.Desc.IsStreamingServer() {
			continue
		}
		var methodDesc []byte
		commentLines := strings.Split(strings.TrimSuffix(method.Comments.Leading.String(), "\n"), "\n")
		commentLinesLen := len(commentLines)
		for index, line := range commentLines {
			methodDesc = append(methodDesc, strings.TrimPrefix(line, "// ")...)
			if index == commentLinesLen-1 {
				methodDesc = append(methodDesc, "."...)
			} else {
				methodDesc = append(methodDesc, ","...)
			}
		}
		httpOptions, ok := proto.GetExtension(method.Desc.Options(), annotations.E_Http).([]*annotations.Http)
		if ok {

			for _, httpOption := range httpOptions {
				g.P("{")
				g.P("MethodName: ", strconv.Quote(string(method.Desc.Name())), ",")
				g.P("Desc: \"", string(methodDesc), "\",")
				switch httpOption.GetPattern().(type) {
				case *annotations.Http_Get:
					g.P("Method: \"", http.MethodGet, "\",")
					g.P("Path: \"", httpOption.GetGet(), "\",")
				case *annotations.Http_Put:
					g.P("Method: \"", http.MethodPut, "\",")
					g.P("Path: \"", httpOption.GetPut(), "\",")
				case *annotations.Http_Post:
					g.P("Method: \"", http.MethodPost, "\",")
					g.P("Path: \"", httpOption.GetPost(), "\",")
				case *annotations.Http_Delete:
					g.P("Method: \"", http.MethodDelete, "\",")
					g.P("Path: \"", httpOption.GetDelete(), "\",")
				case *annotations.Http_Patch:
					g.P("Method: \"", http.MethodPatch, "\",")
					g.P("Path: \"", httpOption.GetPatch(), "\",")
				case *annotations.Http_Head:
					g.P("Method: \"", http.MethodHead, "\",")
					g.P("Path: \"", httpOption.GetHead(), "\",")
				}
				g.P("Handler: ", handlerNames[i], ",")
				g.P("},")
			}
		}
	}
	g.P("},")
	g.P("}")
	g.P()
}

func serverStreamInterface(g *protogen.GeneratedFile, method *protogen.Method) string {
	typeParam := g.QualifiedGoIdent(method.Input.GoIdent) + ", " + g.QualifiedGoIdent(method.Output.GoIdent)
	if method.Desc.IsStreamingClient() && method.Desc.IsStreamingServer() {
		return g.QualifiedGoIdent(restPackage.Ident("BidiStreamingServer")) + "[" + typeParam + "]"
	} else if method.Desc.IsStreamingClient() {
		return g.QualifiedGoIdent(restPackage.Ident("ClientStreamingServer")) + "[" + typeParam + "]"
	} else { // i.e. if method.Desc.IsStreamingServer()
		return g.QualifiedGoIdent(restPackage.Ident("ServerStreamingServer")) + "[" + g.QualifiedGoIdent(method.Output.GoIdent) + "]"
	}
}

func genServerMethod(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile, method *protogen.Method, serverType string, hnameFuncNameFormatter func(string) string) string {
	service := method.Parent
	hname := fmt.Sprintf("_%s_%s_RestHandler", service.GoName, method.GoName)

	g.P("func ", hnameFuncNameFormatter(hname), "(ctx *", restPackage.Ident("Context"), ", srv any, interceptor ", serverPackage.Ident("UnaryServerInterceptor"), ") (any, error) {")
	g.P("in := new(", method.Input.GoIdent, ")")
	g.P("if interceptor == nil {")
	g.P("return srv.(", serverType, ").", method.GoName, "(ctx, in)")
	g.P("}")
	g.P("info := &", serverPackage.Ident("UnaryServerInfo"), "{")
	g.P("Server: srv,")
	g.P("FullMethod: ", service.GoName, "_", method.GoName, "_FullMethodName,")
	g.P("Protocol: ", restPackage.Ident("Protocol"), ",")
	g.P("}")
	g.P("handler := func(ctx ", contextPackage.Ident("Context"), ",req any)(any, error) {")
	g.P("return srv.(", serverType, ").", method.GoName, "(ctx, in)")
	g.P("}")
	g.P("return interceptor(ctx, in, info, handler)")
	g.P("}")
	return hname
}

func genLeadingComments(g *protogen.GeneratedFile, loc protoreflect.SourceLocation) {
	for _, s := range loc.LeadingDetachedComments {
		g.P(protogen.Comments(s))
		g.P()
	}
	if s := loc.LeadingComments; s != "" {
		g.P(protogen.Comments(s))
		g.P()
	}
}

const deprecationComment = "// Deprecated: Do not use."

func unexport(s string) string { return strings.ToLower(s[:1]) + s[1:] }
