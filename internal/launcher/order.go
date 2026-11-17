package launcher

import "log"

// togglePin 切换置顶并落盘：新置顶的排到置顶组最前，取消置顶后回到默认排序
func (s *Server) togglePin(in *instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	in.p.Pin = !in.p.Pin
	if in.p.Pin {
		min := 0
		for _, other := range s.instances {
			if other != in && other.p.Pin && other.p.Order < min {
				min = other.p.Order
			}
		}
		in.p.Order = min - 10
	} else if in.p.Order < 0 {
		in.p.Order = 0
	}
	return s.store.SaveOrder(in.p)
}

// moveInstance 在展示顺序里上移/下移一位，重新编号后写回 apps.json
func (s *Server) moveInstance(in *instance, delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := ordered(s.instances, s.running)
	idx := -1
	for i := range list {
		if list[i] == in {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	// 不跨过置顶分组，避免点了没反应的迷惑行为
	next := idx + delta
	if next < 0 || next >= len(list) || list[next].p.Pin != in.p.Pin {
		return
	}
	list[idx], list[next] = list[next], list[idx]

	for i, item := range list {
		order := (i + 1) * 10
		if item.p.Order == order {
			continue
		}
		item.p.Order = order
		if err := s.store.SaveOrder(item.p); err != nil {
			log.Printf("保存项目 %s 排序失败: %v", item.p.Name, err)
		}
	}
}
