package execution

import (
	"sync"
	"time"
)

type OrderManager struct {
	orders map[string]*Order
	mu     sync.RWMutex
}

func NewOrderManager() *OrderManager {
	return &OrderManager{
		orders: make(map[string]*Order),
	}
}

func (om *OrderManager) AddOrder(order *Order) {
	om.mu.Lock()
	defer om.mu.Unlock()
	om.orders[order.ID] = order
}

func (om *OrderManager) GetOrder(id string) *Order {
	om.mu.RLock()
	defer om.mu.RUnlock()
	return om.orders[id]
}

func (om *OrderManager) processOrderUpdate(data map[string]interface{}) {
	orderID, _ := data["oid"].(string)
	status, _ := data["status"].(string)
	
	om.mu.Lock()
	defer om.mu.Unlock()
	
	if order, exists := om.orders[orderID]; exists {
		order.Status = OrderStatus(status)
		order.UpdatedAt = time.Now()
	}
}
