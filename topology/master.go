package topology

import (
	"github.com/sydnash/lotou/core"
	"github.com/sydnash/lotou/encoding/gob"
	"github.com/sydnash/lotou/log"
	"github.com/sydnash/lotou/network/tcp"
)

type master struct {
	*core.Skeleton
	nodesMap      map[uint]uint //nodeid : agent
	globalNameMap map[string]uint
}

func StartMaster(ip, port string) {
	m := &master{Skeleton: core.NewSkeleton(0)}
	m.nodesMap = make(map[uint]uint)
	m.globalNameMap = make(map[string]uint)
	core.StartService(".master", m)

	s := tcp.New(ip, port, m.Id)
	s.Listen()
}

func (m *master) OnNormalMSG(src uint, data ...interface{}) {
	//dest is master's id, src is core's id
	//data[0] is cmd such as (registerNodeRet, regeisterNameRet, getIdByNameRet...)
	//data[1] is dest nodeService's id
	//parse node's id, and choose correct agent and send msg to that node.
	cmd := data[0].(string)
	if cmd == "syncName" {
		t1 := gob.Pack(data)
		for _, v := range m.nodesMap {
			m.RawSend(v, core.MSG_TYPE_NORMAL, tcp.AGENT_CMD_SEND, t1)
		}
	} else if cmd == "forward" {
		msg := data[1].(*core.Message)
		m.forwardM(msg, nil)
	} else if cmd == "registerName" {
		id := data[1].(uint)
		name := data[2].(string)
		m.onRegisterName(id, name)
	}
}

func (m *master) onRegisterNode(src uint) {
	//generate node id
	nodeId := core.GenerateNodeId()
	m.nodesMap[nodeId] = src
	ret := make([]interface{}, 2, 2)
	ret[0] = "registerNodeRet"
	ret[1] = nodeId
	log.Info("registerNodeRet %v", nodeId)
	sendData := gob.Pack(ret)
	m.RawSend(src, core.MSG_TYPE_NORMAL, tcp.AGENT_CMD_SEND, sendData)
}

func (m *master) onRegisterName(serviceId uint, serviceName string) {
	m.globalNameMap[serviceName] = serviceId
}

func (m *master) onGetIdByName(src uint, name string, rId uint) {
	id, ok := m.globalNameMap[name]
	ret := make([]interface{}, 5, 5)
	ret[0] = "getIdByNameRet"
	ret[1] = id
	ret[2] = ok
	ret[3] = name
	ret[4] = rId
	sendData := gob.Pack(ret)
	m.RawSend(src, core.MSG_TYPE_NORMAL, tcp.AGENT_CMD_SEND, sendData)
}

func (m *master) OnSocketMSG(src uint, data ...interface{}) {
	//dest is master's id, src is agent's id
	//data[0] is socket status
	//data[1] is a gob encode data
	//it's first encode value is cmd such as (registerNode, regeisterName, getIdByName, forword...)
	//it's second encode value is dest service's id.
	//find correct agent and send msg to that node.
	cmd := data[0].(int)
	if cmd == tcp.AGENT_DATA {
		sdata := gob.Unpack(data[1].([]byte))
		array := sdata.([]interface{})
		scmd := array[0].(string)
		if scmd == "registerNode" {
			m.onRegisterNode(src)
		} else if scmd == "registerName" {
			serviceId := array[1].(uint)
			serviceName := array[2].(string)
			m.onRegisterName(serviceId, serviceName)
		} else if scmd == "getIdByName" {
			name := array[1].(string)
			rId := array[2].(uint)
			m.onGetIdByName(src, name, rId)
		} else if scmd == "forward" {
			msg := array[1].(*core.Message)
			m.forwardM(msg, data[1].([]byte))
		}
	} else if cmd == tcp.AGENT_CLOSED {
		var nodeId uint = 0
		for id, v := range m.nodesMap {
			if v == src {
				nodeId = id
			}
		}
		delete(m.nodesMap, nodeId)

		deletedNames := []string{}
		for name, id := range m.globalNameMap {
			nid := core.ParseNodeId(id)
			if nid == nodeId {
				deletedNames = append(deletedNames, name)
			}
		}
		ret := make([]interface{}, 2, 2)
		ret[0] = "nameDeleted"
		ret[1] = deletedNames
		sendData := gob.Pack(ret)
		for _, agent := range m.nodesMap {
			m.RawSend(agent, core.MSG_TYPE_NORMAL, tcp.AGENT_CMD_SEND, sendData)
		}
		core.DistributeMSG(m.Id, ret...)
	}
}

func (m *master) forwardM(msg *core.Message, data []byte) {
	nodeId := core.ParseNodeId(msg.Dst)
	isLcoal := core.CheckIsLocalServiceId(msg.Dst)
	//log.Debug("master forwardM is send to master: %v, nodeid: %d", isLcoal, nodeId)
	if isLcoal {
		core.ForwardLocal(msg)
		return
	}
	agent, ok := m.nodesMap[nodeId]
	if !ok {
		log.Debug("node:%v is disconnected.", nodeId)
		return
	}
	if data == nil {
		ret := make([]interface{}, 2, 2)
		ret[0] = "forward"
		ret[1] = msg
		data = gob.Pack(ret)
	}
	m.RawSend(agent, core.MSG_TYPE_NORMAL, tcp.AGENT_CMD_SEND, data)
}

func (m *master) OnDestroy() {
	for _, v := range m.nodesMap {
		m.SendClose(v)
	}
}
