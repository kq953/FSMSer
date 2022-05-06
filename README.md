# 适用场景
* 使用windows开发linux上比较复杂的web接口，想实时编译看效果；windows与linux采用共享文件系统方式共享代码
* 需要搭配FSMCli一起使用，FSMCli放在编写代码系统中编译运行，FSMSer放在linux执行代码系统中编译运行

# clone方式安装使用
```
cd $GOPATH
go clone https://github.com/kq953/FSMSer.git
cd FSMSer
go install
$GOPATH/bin/FSMSer
```