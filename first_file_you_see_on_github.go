package main

func main() {
	srv := CreateServer()
	// TODO: if you see this on github I forgot to delete it
	panic(srv.myFavField)
	srv.RunServer()
}

type Server struct {
	myFavField string
}

func CreateServer() *Server {
	// this is just a demo
	return nil
}

func (s *Server) RunServer() {
	// this is just a demo
}
