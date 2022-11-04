# 编译调试
{
    alias gob='go build -v -gcflags "all=-N -l" -o filebeat main.go'
    alias dlv='gob && dlv exec ./filebeat --init .dbg/filebeat.dlv'
}
