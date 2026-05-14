package main

import (
	"flag"
	"fmt"

	"a2a-platform/internal/config"
	"a2a-platform/internal/handler"
	"a2a-platform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/host.yaml", "the config file")

func main() {
	flag.Parse()

	c := config.MustLoad(*configFile)
	svcCtx := svc.NewServiceContext(c)

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	handler.RegisterHandlers(server, svcCtx)

	fmt.Printf("A2A Host started on %s:%d\n", c.Host, c.Port)
	server.Start()
}
