package ansible

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"

	"github.com/go-martini/martini"
)

type Server struct {
	m *martini.Martini
}

func NewServer() *Server {
	s := &Server{}
	r := martini.NewRouter()
	r.Get("/ping", s.Ping)
	r.Post("/exec", s.ExecCommand)
	r.Put("/upload", s.PutFile)

	m := martini.New()
	m.Use(martini.Logger())
	m.Use(martini.Recovery())
	m.MapTo(r, (*martini.Routes)(nil))
	m.Action(r.Handle)
	s.m = m
	return s
}

func (s *Server) ConfigureLDAP(options *LdapOptions) {
	s.m.Use(LdapAuthenticator(options))
}

func (s *Server) Serve(l net.Listener) error {
	return http.Serve(l, s.m)
}

func (s *Server) Ping() []byte {
	serverInfo := map[string]string{}
	out, _ := json.Marshal(&serverInfo)
	return out
}

func (s *Server) ExecCommand(req *http.Request) (int, interface{}) {
	command := req.FormValue("command")
	if command == "" {
		return http.StatusInternalServerError, "command is a required parameter\n"
	}

	executable := req.FormValue("executable")
	if executable == "" {
		executable = "/bin/sh"
	}

	become := false
	if arg := req.FormValue("become"); arg != "" {
		value, err := strconv.Atoi(arg)
		if err != nil {
			return http.StatusInternalServerError, fmt.Sprintf("error decoding 'become' value: %s", err)
		}

		if value != 0 {
			become = true
		}
	}

	becomeMethod := req.FormValue("becomeMethod")
	if becomeMethod == "" {
		becomeMethod = "sudo"
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)

	// preallocate the command array (we have a maximum of 5 elements at the moment)
	cmdArgs := make([]string, 0, 5)
	if become {
		switch becomeMethod {
		case "sudo":
			cmdArgs = append(cmdArgs, "sudo", "-n")
		default:
			return http.StatusInternalServerError, fmt.Sprintf("unsupported become method '%s'", becomeMethod)
		}
	}
	cmdArgs = append(cmdArgs, executable, "-c", command)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()

	data := map[string]interface{}{}
	if err != nil {
		data["status"] = 1
	} else {
		data["status"] = 0
	}
	data["stdin"] = ""
	data["stdout"] = stdout.String()
	data["stderr"] = stderr.String()

	out, err := json.Marshal(&data)
	if err != nil {
		return http.StatusInternalServerError, err.Error()
	}
	return http.StatusOK, out
}

func (s *Server) PutFile(req *http.Request) (int, string) {
	dest := req.FormValue("dest")
	src, _, err := req.FormFile("src")
	if err != nil {
		return http.StatusInternalServerError, err.Error()
	}

	f, err := os.Create(dest)
	if err != nil {
		return http.StatusInternalServerError, err.Error()
	}
	defer f.Close()

	io.Copy(f, src)
	return http.StatusOK, ""
}
