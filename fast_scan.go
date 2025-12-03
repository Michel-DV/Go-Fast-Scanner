package main

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
)

// Worker function: prende porte dal canale 'ports' e invia risultati al canale 'results'
func worker(ports, results chan int, wg *sync.WaitGroup, target string) {
	defer wg.Done() // Segnala che il worker ha finito quando esce
	for p := range ports {
		address := fmt.Sprintf("%s:%d", target, p)
		conn, err := net.DialTimeout("tcp", address, 1*time.Second) // Timeout breve
		if err != nil {
			results <- 0 // 0 significa porta chiusa
			continue
		}
		conn.Close()
		results <- p // Invia il numero della porta aperta
	}
}

func main() {
	target := "scanme.nmap.org" // Target di test (cambialo con localhost o un tuo server)
	start := time.Now()

	ports := make(chan int, 100)
	results := make(chan int)
	var openports []int
	var wg sync.WaitGroup

	fmt.Printf("[*] Avvio scansione veloce su %s...\n", target)

	// Avvia 100 worker (Threads leggeri) in parallelo
	for i := 0; i < cap(ports); i++ {
		go worker(ports, results, &wg, target)
		wg.Add(1)
	}

	// Manda le porte da scansionare (1-1024) in un thread separato
	go func() {
		for i := 1; i <= 1024; i++ {
			ports <- i
		}
		close(ports)
	}()

	// Raccoglie i risultati in un thread separato
	go func() {
		for p := range results {
			if p != 0 {
				openports = append(openports, p)
			}
		}
	}()

	wg.Wait()      // Aspetta che tutti i worker finiscano
	close(results) // Chiude il canale risultati

	// Stampa report
	sort.Ints(openports)
	for _, port := range openports {
		fmt.Printf("[+] %d open\n", port)
	}

	fmt.Printf("[*] Scansione completata in %v\n", time.Since(start))
}
