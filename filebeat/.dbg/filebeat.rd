# 编译调试
{
    alias gob='CGO_ENABLED=0 go build -v -gcflags "all=-N -l" -o filebeat main.go'
    alias dlv='gob && dlv exec ./filebeat --init .dbg/filebeat.dlv'
}

# 日志文件
{
    while [[ 1 ]]; do
        date >> /home/qstesiro/github.com/qstesiro/beats/filebeat/demo.log
        sleep 2s
    done
}
