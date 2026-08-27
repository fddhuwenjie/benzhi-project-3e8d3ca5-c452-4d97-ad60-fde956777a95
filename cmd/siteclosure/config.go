package main

import (
	"flag"
	"fmt"
	"net"
	"strconv"
)

type config struct {
	addr     string
	dataDir  string
	selftest bool
}

func parseConfig() (config, error) {
	var cfg config
	flag.StringVar(&cfg.addr, "addr", "127.0.0.1:19081", "HTTP监听地址")
	flag.StringVar(&cfg.dataDir, "data", ".siteclosure-data", "本地数据目录")
	flag.BoolVar(&cfg.selftest, "selftest", false, "运行完整HTTP自检后退出")
	flag.Parse()
	host, portText, err := net.SplitHostPort(cfg.addr)
	if err != nil {
		return cfg, fmt.Errorf("invalid addr: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return cfg, fmt.Errorf("addr must use a loopback IP")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1024 || port > 65535 {
		return cfg, fmt.Errorf("addr must use a high port")
	}
	return cfg, nil
}
