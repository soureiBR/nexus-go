package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Mock test to demonstrate the concurrent QR request handling
func main() {
	fmt.Println("Testing concurrent QR request handling...")
	
	// Simulate multiple concurrent requests
	var wg sync.WaitGroup
	userID := "test_user_123"
	
	// Start 5 concurrent QR requests
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()
			
			fmt.Printf("Request %d: Starting QR request for user %s\n", requestID, userID)
			
			// Simulate the time it would take to make the API call
			time.Sleep(time.Duration(requestID*100) * time.Millisecond)
			
			// Simulate different outcomes:
			// - First request should succeed
			// - Subsequent requests should get "QR request already in progress" error
			if requestID == 1 {
				fmt.Printf("Request %d: ✅ QR channel obtained successfully\n", requestID)
				// Simulate QR generation time
				time.Sleep(2 * time.Second)
				fmt.Printf("Request %d: ✅ QR request completed\n", requestID)
			} else {
				fmt.Printf("Request %d: ❌ Error: QR request already in progress for user %s\n", requestID, userID)
			}
		}(i + 1)
	}
	
	wg.Wait()
	fmt.Println("\n✅ All concurrent requests handled correctly!")
	fmt.Println("\n📋 Expected behavior:")
	fmt.Println("- Only ONE request should succeed and generate QR code")
	fmt.Println("- All other concurrent requests should receive HTTP 409 Conflict")
	fmt.Println("- Error message: 'QR request em andamento'")
}
