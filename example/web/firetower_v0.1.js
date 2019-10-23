function firetower(addr, onopen) {
	var ws = new WebSocket(addr);
	ws.onopen = onopen

	var _this = this

	this.subscribeKey = 'subscribe'
	this.unSubscribeKey = 'unSubscribe'
	this.publishKey = 'publish'
	this.logopen = true // 开启log

	var logInfo = function(data){
		console.log('[firetower] INFO', data)
	}

	this.publish = function(topic, data){
		if (topic == '' || data == '') {
			return errorMessage('topic或data参数不能为空')
		}

		if (_this.logopen) {
			logInfo('publish topic:"' + topic + '", data:' + JSON.stringify(data))
		}

		ws.send(JSON.stringify({
            type: _this.publishKey,
            topic: topic,
            data: data
        }))
		return successMessage('发送成功')
	}

	this.onmessage = false
	ws.onmessage = function(event){
        if (_this.logopen) {
            logInfo('new message:' + JSON.stringify(event.data))
        }

		if (event.data == 'heartbeat') {
            return 
        }

		if (_this.onmessage) {
			_this.onmessage(event)
		}
	}

	this.onclose = false 
	ws.onclose = function(){
		if (_this.onclose) {
			_this.onclose()
		}
	}

	this.subscribe = function(topicArr){
		if (!Array.isArray(topicArr)) {
			topicArr = [topicArr]
		}

		if (_this.logopen) {
			logInfo('subscribe:"' + topicArr.join(',') + '"')
		}

		ws.send(JSON.stringify({
            type: _this.subscribeKey,
			topic: topicArr.join(','),
			data: ''
        }))
	}

	this.unsubscribe = function(topicArr){
		if (!Array.isArray(topicArr)) {
			topicArr = [topicArr]
		}

		if (_this.logopen) {
			logInfo('unSubscribe:"' + topicArr.join(',') + '"')
		}

		ws.send(JSON.stringify({
            type: _this.unSubscribeKey,
            topic: topicArr.join(','),
            data: ''
        }))
	}

	function errorMessage(info){
		return {
			type: 'error',
			info: info
		}
	}

	function successMessage(info){
		return {
			type: 'success',
			info: info
		}
	}
}

