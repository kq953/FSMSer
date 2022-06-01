package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"proto"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	wg   sync.WaitGroup
	port = ":8099"
	//上传队列
	proStrChan chan string
	//执行cmd
	cmdList = map[string]*exec.Cmd{}
	//关闭通道
	stopList = map[string]chan string{}
	//连接
	connList []net.Conn
	//输出
	outList []io.ReadCloser
)

func main() {
	proStrChan = make(chan string, 50)

	stopChan := make(chan os.Signal, 1) //接收系统中断信号
	signal.Notify(stopChan, os.Interrupt, os.Kill)

	listen, err := net.Listen("tcp", port)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	defer listen.Close()

	//关闭操控
	go func() {
		<-stopChan
		fmt.Println("正在关闭")
		//关闭客户端连接
		for _, c := range connList {
			_ = c.Close()
		}
		//关闭监听服务
		_ = listen.Close()
		//关闭
		for _, o := range outList {
			_ = o.Close()
		}
		//关闭正在执行命令
		for _, cmd := range cmdList {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		close(stopChan)
		close(proStrChan)
	}()

	//命令执行
	wg.Add(1)
	go execCommand()

	fmt.Println("服务已启动[" + port + "]...")
	for {
		conn, err := listen.Accept()
		if err != nil {
			//fmt.Println("连接已关闭")
			break
		}
		fmt.Println(conn.RemoteAddr().String(), "已连接")
		connList = append(connList, conn)
		wg.Add(1)
		go handler(conn)
	}
	wg.Wait()
	fmt.Println("已退出")
}

//处理方法
func handler(conn net.Conn) {
	defer func() {
		wg.Done()
		conn.Close()
	}()

	reader := bufio.NewReader(conn)
	for {
		revStr, err := proto.Decode(reader)
		if err != nil {
			fmt.Println("读取失败:", err.Error())
			break
		}

		info := strings.Split(revStr, ":")
		if len(info) < 2 {
			fmt.Println("数据格式异常:", revStr)
			break
		}
		//根据协议处理
		switch info[0] {
		case "RERUN": //重新编译项目
			proStrChan <- info[1]
		default:
			fmt.Println("错误协议请求")
		}
	}
}

//执行窗口编译命令
func execCommand() {
	defer wg.Done()
	proArr := make(map[string]int8)
	for {
		proName, ok := <-proStrChan
		if !ok {
			return
		}
		proArr[proName] = 1
		//等待1秒，去除多次无效的重复编译
		time.Sleep(time.Millisecond * 500)
		if l := len(proStrChan); l > 0 {
			for i := 0; i < l; i++ {
				proName = <-proStrChan
				if _, ok := proArr[proName]; !ok {
					proArr[proName] = 1
				}
			}
		}
		//fmt.Println("stopList:", stopList[proName])
		for proName = range proArr {
			if stopList[proName] == nil {
				stopList[proName] = make(chan string, 1)
			}
			if cmd, ok := cmdList[proName]; ok {
				//fmt.Println("kill:",cmd.Process.Pid)
				//err := cmd.Process.Kill()
				//if err != nil {
				//	fmt.Println("Kill err:", err.Error())
				//}
				syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				delete(cmdList, proName)
				//等待关闭
				<-stopList[proName]
			}
			fmt.Println("\n\n目录：", proName)
			wg.Add(1)
			go exeShell(proName)
		}
		//清空数据
		proArr = make(map[string]int8)
	}
}

//开始执行shell
func exeShell(proName string) {
	defer func() {
		wg.Done()
		stopList[proName] <- "stop"
	}()

	var mainFile strings.Builder
	mainFile.WriteString(os.Getenv("GOPATH"))
	mainFile.WriteString("/src/")
	mainFile.WriteString(proName)
	mainFile.WriteString("/main.go")
	_, err := os.Stat(mainFile.String())
	if err != nil {
		fmt.Println(proName, "项目无main.go文件 忽略")
		return
	}

	var cmdStr strings.Builder
	//cd 到项目目录
	cmdStr.WriteString("cd ")
	cmdStr.WriteString(os.Getenv("GOPATH"))
	cmdStr.WriteString("/src/")
	cmdStr.WriteString(proName)
	cmdStr.WriteString("; go run main.go;")

	cmd := exec.Command("/bin/bash", "-c", cmdStr.String())

	//ctx := context.Background()
	//cmd := exec.CommandContext(ctx, "/bin/bash", "-c", cmdStr.String())

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, _ := cmd.StdoutPipe()
	errOut, _ := cmd.StderrPipe()

	outList = append(outList, out)
	fmt.Println(time.Now().Format("2006-01-02 15:04:05"), " building...")
	err = cmd.Start()
	if err != nil {
		fmt.Println("编译启动失败:", err.Error())
		return
	}
	//异步打印
	wg.Add(2)
	go syncPrint(out)
	go syncPrint(errOut)
	cmdList[proName] = cmd
	//fmt.Println("pid:", cmd.Process.Pid)
	err = cmd.Wait()
	if err != nil {
		fmt.Println("stop wait:", err.Error())
	}
}

//打印输出
func syncPrint(reader io.ReadCloser) {
	defer wg.Done()

	buf := make([]byte, 1024)
	for {
		num, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return
		}
		if num > 0 {
			fmt.Print(string(buf[:num]))
		}
	}
}
