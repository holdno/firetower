

<p align="center">
  <a href="https://github.com/holdno/beacontower/archive/master.zip"><img src="https://img.shields.io/badge/download-fast-brightgreen.svg" alt="Downloads"></a>
  <img src="https://img.shields.io/badge/build-passing-brightgreen.svg" alt="Build Status">
  <img src="https://img.shields.io/badge/package%20utilities-dep-blue.svg" alt="Package Utilities">
  <img src="https://img.shields.io/badge/golang-1.10.0-%23ff69b4.svg" alt="Version">
  <img src="https://img.shields.io/badge/issue-waiting-red.svg" alt="Issue">
  <img src="https://img.shields.io/badge/tests-3%20Demo-ff69b4.svg" alt="Demo">
</p>
<h1 align="center">Beacontower</h2>
分布式推送服务  

基于websocket封装，围绕topic进行sub/pub    
体验地址: http://chat.ojbk.io  
### 构成

基本服务由两点构成
- topic管理服务
> 详见示例 example/topicService  

该服务主要作为集群环境下唯一的topic管理节点
beacontower一定要依赖这个管理节点才能正常工作
- websocket服务
> 详见示例 example/websocketService  

websocket服务是用户基于beacontower自定义开发的业务逻辑
可以通过beacontower提供的回调方法来实现自己的业务逻辑
（web client 在 example/web 下)  
### 接入姿势  
``` golang 
package main

import (
    "fmt"
    "github.com/gorilla/websocket"
    "github.com/holdno/beacontower/gateway"
    "github.com/holdno/snowFlakeByGo" // 这是一个分布式全局唯一id生成器
    "net/http"
    "strconv"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
} 

var GlobalIdWorker *snowFlakeByGo.Worker

func main() {
    GlobalIdWorker, _ = snowFlakeByGo.NewWorker(1) 
    http.HandleFunc("/ws", Websocket)
    http.ListenAndServe("0.0.0.0:9999", nil)
}

func Websocket(w http.ResponseWriter, r *http.Request) {
    // 做用户身份验证
    ...
    // 验证成功才升级连接
    ws, _ := upgrader.Upgrade(w, r, nil)
    // 生成一个全局唯一的clientid
    id := GlobalIdWorker.GetId()
    tower := gateway.BuildTower(ws, strconv.FormatInt(id, 10)) // 生成一个烽火台
    tower.Run()
}
```
目前支持的回调方法：
- ReadHandler 收到客户端发送的消息时触发
``` golang
tower := gateway.BuildTower(ws, strconv.FormatInt(id, 10)) // 创建beacontower实例
tower.SetReadHandler(func(message *gateway.TopicMessage) bool { // 绑定ReadHandler回调方法
    // message.Data 为客户端传来的信息
    // message.Topic 为消息传递的topic
    // 用户可在此做发送验证
    // 判断发送方是否有权限向到达方发送内容
    // 通过 Publish 方法将内容推送到所有订阅 message.Topic 的连接
    tower.Publish(message)
    return true
})
```

- BeforeSubscribeHandler 客户端订阅某些topic时触发(这个时候topic还没有订阅，是before subscribe)
``` golang
tower.SetBeforeSubscribeHandler(func(topic []string) bool {
    // 这里用来判断当前用户是否允许订阅该topic
    return true // 返回true则进行正常流程 false则终止
})
```

- SubscribeHandler 客户端完成某些topic的订阅时触发(topic已经被topicService收录并管理)
``` golang
tower.SetSubscribeHandler(func(topic []string) bool {
    // 我们给出的聊天室示例是需要用到这个回调方法
    // 当某个聊天室(topic)有新的订阅者，则需要通知其他已经在聊天室内的成员当前在线人数+1
    for _, v := range topic {
        num := tower.GetConnectNum(v)

        var pushmsg = new(gateway.TopicMessage)
        pushmsg.Topic = v
        pushmsg.Data = []byte(fmt.Sprintf("{\"type\":\"onSubscribe\",\"data\":%d}", num))
        tower.Publish(pushmsg)
    }
    return true // 返回true则进行正常流程 false则终止
})
```

- UnSubscribeHandler 客户端取消订阅某些topic完成时触发 (这个回调方法没有设置before方法，目前没有想到什么场景会使用到before unsubscribe，如果有请issue联系)
``` golang
tower.SetUnSubscribeHandler(func(topic []string) bool {
    // 当某个聊天室(topic)有人退出，则需要通知其他已经在聊天室内的成员当前在线人数-1
    for _, v := range topic {
        num := tower.GetConnectNum(v)
        var pushmsg = new(gateway.TopicMessage)
        pushmsg.Topic = v
        pushmsg.Data = []byte(fmt.Sprintf("{\"type\":\"onUnsubscribe\",\"data\":%d}", num))
        tower.Publish(pushmsg)
    }

    return true
})
```
注意：当客户端断开websocket连接时beacontower会将其在线时订阅的所有topic进行退订 会触发UnSubscirbeHandler  

## TODO
小包合并大包，减少发送频率    
