package proxys

import (
	"sync"
	"time"
)

type IProxy struct {
	ips  []string
	bad  map[string]time.Time // 出口 -> 冷却截止时间
	lock sync.Mutex
}

func NewIProxyIP(ips []string) IProxy {
	return IProxy{
		ips: ips,
		bad: map[string]time.Time{},
	}
}

func (p *IProxy) GetIPS() int {
	return len(p.ips)
}

// GetProxyIP 轮询取下一个不在冷却期的出口; 全部冷却时取最早解冻的那个.
func (p *IProxy) GetProxyIP() string {
	if p == nil {
		return ""
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	if len(p.ips) == 0 {
		return ""
	}

	now := time.Now()
	best := 0
	for i, ip := range p.ips {
		until, cooling := p.bad[ip]
		if !cooling || now.After(until) {
			best = i
			break
		}
		if until.Before(p.bad[p.ips[best]]) {
			best = i
		}
	}
	proxyIp := p.ips[best]
	p.ips = append(append(p.ips[:best:best], p.ips[best+1:]...), proxyIp)
	return proxyIp
}

// MarkBad 让出口冷却 d; 被 duck.ai 418/429 的 IP 短时间内再打只会继续 418.
func (p *IProxy) MarkBad(ip string, d time.Duration) {
	if p == nil || ip == "" {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.bad == nil {
		p.bad = map[string]time.Time{}
	}
	p.bad[ip] = time.Now().Add(d)
}
