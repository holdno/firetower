# Beacontower
[![](https://img.shields.io/badge/download-fast-brightgreen.svg)](https://github.com/holdno/beacontower/archive/master.zip)
[](https://img.shields.io/badge/build-passing-brightgreen.svg)
分布式推送服务

基于websocket封装，围绕topic进行sub/pub
## 构成

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
目前支持的回调方法：
- ReadHandler 收到客户端发送的消息时触发
``` golang
tower := gateway.BuildTower(ws, strconv.FormatInt(id, 10)) // 创建beacontower实例
tower.SetReadHandler(func(message *gateway.TopicMessage) bool { // 绑定ReadHandler回调方法
    fmt.Println(message.Data)
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
        pushmsg.Data = fmt.Sprintf("{\"type\":\"onSubscribe\",\"data\":%d}", num)
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
        pushmsg.Data = fmt.Sprintf("{\"type\":\"onUnsubscribe\",\"data\":%d}", num)
        tower.Publish(pushmsg)
    }

    return true
})
```
注意：当客户端断开websocket连接时beacontower会将其在线时订阅的所有topic进行退订 会触发UnSubscirbeHandler
