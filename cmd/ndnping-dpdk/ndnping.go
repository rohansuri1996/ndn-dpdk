package main

import (
	"log"
	"time"

	"ndn-dpdk/app/ndnping"
	"ndn-dpdk/appinit"
)

func main() {
	appinit.InitEal()
	pc, e := parseCommand(appinit.Eal.Args[1:])
	if e != nil {
		appinit.Exitf(appinit.EXIT_BAD_CONFIG, "parseCommand: %v", e)
	}

	var clients []ndnping.Client
	for _, clientCfg := range pc.clients {
		face, e := appinit.NewFaceFromUri(clientCfg.face)
		if e != nil {
			appinit.Exitf(appinit.EXIT_FACE_INIT_ERROR, "NewFaceFromUri(%s): %v", clientCfg.face, e)
		}

		client, e := ndnping.NewClient(face)
		if e != nil {
			appinit.Exitf(appinit.EXIT_FACE_INIT_ERROR, "ndnping.NewClient(%s): %v", clientCfg.face, e)
		}
		client.SetInterval(clientCfg.interval)
		for _, pattern := range clientCfg.patterns {
			client.AddPattern(pattern.prefix, pattern.pct)
		}
		client.EnableRtt(8, 16)

		clients = append(clients, client)
	}

	var servers []ndnping.Server
	for _, serverCfg := range pc.servers {
		face, e := appinit.NewFaceFromUri(serverCfg.face)
		if e != nil {
			appinit.Exitf(appinit.EXIT_FACE_INIT_ERROR, "NewFaceFromUri(%s): %v", serverCfg.face, e)
		}

		server, e := ndnping.NewServer(face)
		if e != nil {
			appinit.Exitf(appinit.EXIT_FACE_INIT_ERROR, "ndnping.NewServer(%s): %v", serverCfg.face, e)
		}
		for _, prefix := range serverCfg.prefixes {
			server.AddPattern(prefix)
		}
		servers = append(servers, server)
	}

	for i, server := range servers {
		lc := appinit.MustLaunch(server.Run, server.GetFace().GetNumaSocket())
		log.Printf("server(%d) lcore %d socket %d", i, lc, lc.GetNumaSocket())
	}
	time.Sleep(100 * time.Millisecond)
	for i, client := range clients {
		lc1 := appinit.MustLaunch(client.RunRx, client.GetFace().GetNumaSocket())
		lc2 := appinit.MustLaunch(client.RunTx, lc1.GetNumaSocket())
		log.Printf("client(%d) lcore %d, %d socket %d", i, lc1, lc2, lc1.GetNumaSocket())
	}

	tick := time.Tick(pc.counterInterval)
	go func() {
		for {
			<-tick
			for _, client := range clients {
				face := client.GetFace()
				log.Printf("client(%d) %v; %v", face.GetFaceId(),
					client.ReadCounters(), face.ReadCounters())
			}
			for _, server := range servers {
				face := server.GetFace()
				log.Printf("server(%d) %v; %v", face.GetFaceId(),
					server.ReadCounters(), face.ReadCounters())
			}
		}
	}()

	select {}
}
