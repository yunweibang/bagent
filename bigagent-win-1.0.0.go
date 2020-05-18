package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/imroc/req"
	"github.com/kardianos/service"
	//"github.com/nxsre/lumberjack"
	"github.com/shirou/gopsutil/process"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"gopkg.in/ini.v1"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

func GetCurrentPath() (string, error) {
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	i := strings.LastIndex(path, "/")
	if i < 0 {
		i = strings.LastIndex(path, "\\")
	}
	if i < 0 {
		return "", errors.New(`error: Can't find "/" or "\".`)
	}
	return string(path[0 : i+1]), nil
}

var spacePattern = regexp.MustCompile("\\s+")
var conf Config

var logger service.Logger

type program struct{}

func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	go p.run()
	return nil
}
func (p *program) run() {
	parseCfg()
	//fmt.Printf("%+v", conf)
	//fmt.Println(time.Now().Format("2006-01-02 03:04:05 PM"), "Service started")
	r := req.New()
	r.SetFlags(req.Lcost | req.LrespBody | req.LrespHead)
	//r.SetFlags(req.LstdFlags)
	r.SetTimeout(10 * time.Second)
	serverAddr := ""
	if conf.ProxyIP != "" && conf.ProxyPort != "" && conf.ProxyProto != "" {
		serverAddr = conf.ProxyProto + "://" + conf.ProxyIP + ":" + conf.ProxyPort
	}
	wg := sync.WaitGroup{}
	for {
		wg.Add(1)
		go func() {
			resp, err := r.Post(serverAddr+"/api/agent/tasklist", req.Param{
				"id":       conf.HostID,
				"ak":       conf.HostAK,
				"tasklist": listAllProcess(),
			})
			log.Println(resp, err)
			wg.Done()
		}()
		wg.Add(1)

		go func() {
			resp, err := r.Post(serverAddr+"/api/agent/netstat", req.Param{
				"id":      conf.HostID,
				"ak":      conf.HostAK,
				"type":    "ip",
				"netstat": listAllNetstat(),
			})
			log.Println(resp, err)
			wg.Done()
		}()
		wg.Add(1)
		go func() {
			resp, err := r.Post(serverAddr+"/api/agent/cron", req.Param{
				"id":           conf.HostID,
				"ak":           conf.HostAK,
				"cron":         listAllSchtasks(),
				"crond_status": "1",
			})
			log.Println(resp, err)
			wg.Done()
		}()
		wg.Add(1)
		go func() {
			resp, err := r.Post(serverAddr+"/api/agent/version", req.Param{
				"id":            conf.HostID,
				"ak":            conf.HostAK,
				"agent_version": version(),
			})
			log.Println(resp, err)
			wg.Done()
		}()
		wg.Add(1)
		go func() {
			resp, err := r.Post(serverAddr+"/api/agent/zabbixagent/version", req.Param{
				"id":                conf.HostID,
				"ak":                conf.HostAK,
				"zbx_agent_version": zabbixVersion(),
			})
			log.Println(resp, err)
			wg.Done()
		}()
		wg.Wait()
		time.Sleep(300 * time.Second)
	}
}
func (p *program) Stop(s service.Service) error {
	return nil
}

var help = `bigagent version %s
	install		Install bigagent service
	uninstall 	Uninstall bigagent service
	start		Start bigagent service
	stop		Stop bigagent service
	restart		Restart bigagent service
	run 		Run bigagent in foreground
	/v|-v|-V 	Display version number`

func main() {
	svcConfig := &service.Config{
		Name:        "bigagent",
		DisplayName: "bigagent",
		Description: "bigagent",
		Arguments:   []string{"run"},
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	logger, err = s.Logger(nil)
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			if err:=s.Install();err!=nil{
				log.Fatalln(err)
			}
			log.Println("Service installed successfully.")
		case "uninstall":
			if err:=s.Stop();err!=nil{
				log.Println(err)
			}
			if err:=s.Uninstall();err!=nil{
				log.Fatalln(err)
			}
			log.Println("Service uninstalled successfully.")
		case "start":
			if err:=s.Start();err!=nil{
				log.Fatalln(err)
			}
			log.Println("Service started successfully.")
		case "stop":
			if err:=s.Stop();err!=nil{
				log.Fatalln(err)
			}
			log.Println("Service stoped successfully.")
		case "restart":
			if err:=s.Restart();err!=nil{
				log.Fatalln(err)
			}
			log.Println("Service restart successfully.")
		case "run":
			if err:=s.Run();err!=nil{
				log.Fatalln(err)
			}
		case "/v", "-v", "-V":
			fmt.Println(version())
		default:
			fmt.Printf(help,version())
		}
		return
	}
	fmt.Println(s.Start())
}

type Config struct {
	HostID     string
	HostAK     string
	ProxyProto string
	ProxyIP    string
	ProxyPort  string
}

func version() string {
	return "1.0.0"
}

func GbkToUtf8(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.GBK.NewDecoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func Utf8ToGbk(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.GBK.NewEncoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func parseCfg() {
	p, _ := GetCurrentPath()
	//log.SetOutput(&lumberjack.Logger{
	//	Filename:   filepath.Join(p, "bigagent.log"),
	//	TimeFormat: "2006-01-02T15-04",
	//	MaxSize:    -1,
	//	MaxAge:     0,
	//	MaxBackups: 0,
	//	LocalTime:  false,
	//	Compress:   true,
	//})
	cfg, err := ini.Load(filepath.Join(p, "bigagent.conf"))
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}
	conf.HostID = cfg.Section("").Key("host_id").String()
	conf.HostAK = cfg.Section("").Key("host_ak").String()
	conf.ProxyProto = cfg.Section("").Key("proxy_proto").String()
	conf.ProxyIP = cfg.Section("").Key("proxy_ip").String()
	conf.ProxyPort = cfg.Section("").Key("proxy_port").String()
}

func zabbixVersion() string {
	version := "not_running"
	ps, err := process.Processes()
	if err != nil {
		log.Fatalln(err)
	}
	for _, p := range ps {
		exe, err := p.Exe()
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(exe), "zabbix_agentd.exe") {
			cmd := exec.Command(exe, "-V")
			out, _ := cmd.CombinedOutput()
			lines := strings.Split(string(out), "\r\n")
			vs := strings.Split(lines[0], " ")
			version = vs[len(vs)-1]
		}
	}
	return version
}

func listAllSchtasks() string {
	cmd := exec.Command("schtasks", "/query")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	data := strings.Split(string(out), "\r\n")

	bs := strings.Join(data[1:len(data)-1], "\r\n")
	d, _ := GbkToUtf8([]byte(bs))
	return string(d)
}

func listAllNetstat() string {
	cmd := exec.Command("netstat", "-ano")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	netstats := strings.Split(string(out), "\r\n")
	data := []string{}
	for _, n := range netstats[4 : len(netstats)-1] {
		if !strings.Contains(n, "*:*") {
			src := spacePattern.Split(n, -1)
			line := append(src[1:2], "0", "0", strings.Join(src[2:], " "))
			data = append(data, strings.Join(line, " "))
		}
	}
	bs := strings.Join(data, "\n")
	d, _ := GbkToUtf8([]byte(bs))
	return string(d)
}

func listAllProcess() string {
	cmd := exec.Command("tasklist", "/FO", "csv", "/NH")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	all, _ := csv.NewReader(bytes.NewReader(out)).ReadAll()
	data := []string{}
	for _, line := range all {
		data = append(data, strings.Join(line, "||"))
	}
	bs := strings.Join(data, "\r\n")
	d, _ := GbkToUtf8([]byte(bs))
	return string(d)
}
