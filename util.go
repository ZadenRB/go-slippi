package slippi

func MakeUnboundedChannel[K any]() (chan<- *K, <-chan *K) {
	in := make(chan *K)
	out := make(chan *K)

	go func() {
		var sendQueue []*K
		outCh := func() chan *K {
			if len(sendQueue) == 0 {
				return nil
			}
			return out
		}
		toSend := func() *K {
			if len(sendQueue) == 0 {
				return nil
			}
			return sendQueue[0]
		}

		for len(sendQueue) > 0 || in != nil {
			select {
			case e, ok := <-in:
				if !ok {
					in = nil
				} else {
					sendQueue = append(sendQueue, e)
				}
			case outCh() <- toSend():
				sendQueue = sendQueue[1:]
			}
		}
		close(out)
	}()

	return in, out
}
