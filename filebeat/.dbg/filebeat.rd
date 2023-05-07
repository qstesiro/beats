# 编译调试
{
    alias gob='CGO_ENABLED=0 go build -v -gcflags "all=-N -l" -o filebeat main.go'
    # log
    alias dlv='gob && dlv exec ./filebeat --init .dbg/filebeat.dlv -- -e -c ./filebeat.yml'
    # k8s
    alias dlv='export KUBECONFIG=~/.kube/config && gob && dlv exec ./filebeat --init .dbg/filebeat.dlv -- -e -c ./k8s-beat.yml'

    ./filebeat -e -c ./filebeat.yml
}

# 日志文件
{
    while [[ 1 ]]; do
        date >> /home/qstesiro/github.com/qstesiro/beats/filebeat/demo-01.log
        sleep 2s
    done

    while [[ 1 ]]; do
        date >> /home/qstesiro/github.com/qstesiro/beats/filebeat/demo-02.log
        sleep 2s
    done
}

# 远程
{
    kubectl --context=qd-hongdao-test -n logging cp $GOPATH/bin/dlv filebeat-beat-filebeat-fns8p:/bin/

    kubectl --context=qd-hongdao-test -n logging cp filebeat filebeat-beat-filebeat-fns8p:/usr/share/filebeat/filebeat-dev
    kubectl --context=qd-hongdao-test -n logging cp filebeat-qd-hongdao-test.yml filebeat-beat-filebeat-fns8p:/usr/share/filebeat/

    kubectl --context=qd-hongdao-test -n logging exec -it pod/filebeat-beat-filebeat-fns8p -- sh

    chown root filebeat-qd-hongdao-test.yml
    chmod go-w filebeat-qd-hongdao-test.yml

    dlv attach 7 -l=:1025 --headless=true --api-version=2
    dlv exec ./filebeat-dev -l=:1025 --headless=true --api-version=2 -- -e -c filebeat-qd-hongdao-test.yml --path.data data-dev --path.logs logs-dev
    kubectl --context=qd-hongdao-test -n logging port-forward pod/filebeat-beat-filebeat-fns8p 1025:1025
    alias dlv='dlv connect localhost:1025 --init .dbg/filebeat.dlv'

    # ./filebeat-dev -e -c filebeat-qd-hongdao-test.yml --path.data data01 --path.logs logp
}

# docker
{
    # 修改dockerhub域名
    docker build ./ -f Dockerfile.dev -t reg.xxx.net/release/filebeat-dev:7.13.4.18
    docker push reg.xxx.net/release/filebeat-dev:7.13.4.18
    kubectl --context=qd-aliyun-dmz-ack-test -n logging edit daemonset/filebeat-beat-filebeat

    # 更新image
    kubectl --context=qd-aliyun-dmz-ack-test edit daemon/filebeat-beat-filebeat
    kubectl --context=qd-aliyun-dmz-ack-test get pod -o wide -A | grep filebeat | grep test-dmz-ack-23238-worker
}

------------------- selector
service
seccomp
console
crawler
autodiscover
input
beat
processors
publisher_pipeline_output
autodiscover.pod
publisher_processing
index-management
kafka
mgmt
modules
filebeat
registrar
acker
centralmgmt
registry
event
kubernetes

{
    0  0x000000000121874f in github.com/elastic/beats/v7/libbeat/publisher/queue/memqueue.(*openState).publish
    at /home/qstesiro/github.com/qstesiro/beats/libbeat/publisher/queue/memqueue/produce.go:132
    1  0x00000000012182f8 in github.com/elastic/beats/v7/libbeat/publisher/queue/memqueue.(*ackProducer).Publish
    at /home/qstesiro/github.com/qstesiro/beats/libbeat/publisher/queue/memqueue/produce.go:88
    2  0x00000000011c4275 in github.com/elastic/beats/v7/libbeat/publisher/pipeline.(*client).publish
    at /home/qstesiro/github.com/qstesiro/beats/libbeat/publisher/pipeline/client.go:134
    3  0x00000000011c3c5e in github.com/elastic/beats/v7/libbeat/publisher/pipeline.(*client).Publish
    at /home/qstesiro/github.com/qstesiro/beats/libbeat/publisher/pipeline/client.go:80
    4  0x000000000247681c in github.com/elastic/beats/v7/filebeat/beater.(*countingClient).Publish
    at ./beater/channels.go:136
    5  0x000000000237985e in github.com/elastic/beats/v7/filebeat/channel.(*outlet).OnEvent
    at ./channel/outlet.go:58
    6  0x000000000237afc7 in github.com/elastic/beats/v7/filebeat/channel.SubOutlet.func1
    at ./channel/util.go:45
    7  0x000000000046f721 in runtime.goexit
    at /home/qstesiro/.gvm/gos/go1.18.8/src/runtime/asm_amd64.s:1571

    interface {}(github.com/elastic/beats/v7/filebeat/input/file.State) {
	    Id: "native::1179681-2053",
	    PrevId: "",
	    Finished: false,
	    Fileinfo: io/fs.FileInfo(*os.fileStat) ...,
	    Source: "/home/qstesiro/github.com/qstesiro/beats/filebeat/demo-01.log",
	    Offset: 5504,
	    Timestamp: (*time.Time)(0xc0000ede90),
	    TTL: -1,
	    Type: "log",
	    Meta: map[string]string nil,
	    FileStateOS: (*"github.com/elastic/beats/v7/libbeat/common/file.StateOS")(0xc0000edec8),
	    IdentifierName: "native",}

    # 设置非活动清理超时后
    # clean_inactive: 2m
    # ignore_older: 1m
    2023-05-04T14:27:28.352+0800	ERROR	file/states.go:125	State for /home/qstesiro/github.com/qstesiro/beats/filebeat/demo-02.log should have been dropped, but couldn't as state is not finished.
2023-05-04T14:27:28.532+0800	INFO	log/harvester.go:342	File is inactive: /home/qstesiro/github.com/qstesiro/beats/filebeat/demo-02.log. Closing because close_inactive of 5m0s reached.
2023-05-04T14:27:28.532+0800	INFO	log/harvester.go:274	-------------------- return read file: /home/qstesiro/github.com/qstesiro/beats/filebeat/demo-02.log, count: 43
}
