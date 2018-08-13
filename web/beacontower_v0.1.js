function beacontower(addr, onopen) {
	var ws = new WebSocket(addr);
	ws.onopen = onopen

	var _this = this

	this.subscribeKey = 'subscribe'
	this.unSubscribeKey = 'unSubscribeKey'
	this.publishKey = 'publish'
	this.messageSplitKey = '|=|'
	this.logopen = true // 开启log

	var logInfo = function(data){
		console.log('[beacontower] INFO', data)
	}

	this.publish = function(topic, data){
		if (topic == '' || data == '') {
			return errorMessage('topic或data参数不能为空')
		}

		if (_this.logopen) {
			logInfo('publish topic:"' + topic + '", data:' + JSON.stringify(data))
		}

		ws.send([_this.publishKey, topic, JSON.stringify(data)].join(_this.messageSplitKey))
		return successMessage('发送成功')
	}

	this.onmessage = false
	ws.onmessage = function(event){
		if (event.data == 'heartbeat from beacontower') {
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

		ws.send([_this.subscribeKey, topicArr.join(','), ''].join(_this.messageSplitKey))
	}

	this.unsubscribe = function(topicArr){
		if (!Array.isArray(topicArr)) {
			topicArr = [topicArr]
		}

		if (_this.logopen) {
			logInfo('unsubscribe:"' + topicArr.join(',') + '"')
		}

		ws.send([_this.unSubscribeKey, topicArr.join(','), ''].join(_this.messageSplitKey))
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

