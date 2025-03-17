package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Проверка реализации интерфейса PipelineStage
// ConsoleConsumer и ConsoleSource не будут реализовывать интерфейс PipelineStage
var _ PipelineStage = &Filter{}
var _ PipelineStage = &RingBuffer{}

const bufferSize = 5
const tickTTL = 10 * time.Second

// PipelineStage - интерфейс для всех стадий обработки
type PipelineStage interface {
	Process(input <-chan int, done <-chan struct{}) <-chan int
}

// Filter - стадия фильтрации чисел по заданным условиям
type Filter struct {
	predicate func(int) bool
}

// NewFilter - создает фильтр с условием
func NewFilter(predicate func(int) bool) *Filter {
	log.Println("Создание NewFilter")
	return &Filter{predicate: predicate}
}

// Process - выполняет фильтрацию данных
func (f *Filter) Process(input <-chan int, done <-chan struct{}) <-chan int {
	output := make(chan int)
	go func() {
		defer close(output)
		for {
			select {
			case <-done:
				return
			case v, ok := <-input:
				if !ok {
					return
				}
				if f.predicate(v) {
					output <- v
				}
			}
		}
	}()
	log.Println("Process Filter")
	return output
}

// RingBuffer - стадия буферизации
type RingBuffer struct {
	size   int
	buffer []int
	mu     sync.Mutex
}

// NewRingBuffer - создает новый буфер
func NewRingBuffer(size int) *RingBuffer {
	log.Println("Создание NewRingBuffer")
	return &RingBuffer{
		size:   size,
		buffer: make([]int, 0, size),
	}
}

// Process - выполняет буферизацию данных
func (rb *RingBuffer) Process(input <-chan int, done <-chan struct{}) <-chan int {
	output := make(chan int)
	go func() {
		defer close(output)
		ticker := time.NewTicker(tickTTL)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case v, ok := <-input:
				if !ok {
					rb.flush(output)
					return
				}
				rb.add(v)
			case <-ticker.C:
				rb.flush(output)
			}
		}
	}()
	log.Println("Process RingBuffer")
	return output
}

func (rb *RingBuffer) add(value int) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if len(rb.buffer) == rb.size {
		log.Println("Буфер переполнен")
		rb.buffer = rb.buffer[1:] // удаляем старейший элемент
	}
	rb.buffer = append(rb.buffer, value)
	log.Printf("Добавлено в буфер: %d\n", value)
}

func (rb *RingBuffer) flush(output chan int) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, v := range rb.buffer {
		output <- v
	}
	rb.buffer = rb.buffer[:0]
	log.Println("Буфер очищен")
}

// ConsoleSource - источник данных
type ConsoleSource struct{}

// Process - читает данные из консоли
func (cs *ConsoleSource) Process(done <-chan struct{}) <-chan int {
	output := make(chan int)
	go func() {
		defer close(output)
		scanner := bufio.NewScanner(os.Stdin)
		for {
			if !scanner.Scan() {
				return
			}
			input := scanner.Text()
			if strings.ToLower(input) == "exit" {
				return
			}
			num, err := strconv.Atoi(strings.TrimSpace(input))
			if err != nil {
				log.Println("Ошибка ввода. Пожалуйста, ввведите число")
				continue
			}
			log.Printf("Введено число: %d\n", num)
			select {
			case <-done:
				return
			case output <- num:
			}
		}
	}()
	log.Println("Process ConsoleSource")
	return output
}

// ConsoleConsumer - потребитель данных
type ConsoleConsumer struct {
	wg sync.WaitGroup
}

// Process - выводит данные в консоль
func (cc *ConsoleConsumer) Process(input <-chan int, done <-chan struct{}) {
	cc.wg.Add(1)
	go func() {
		defer cc.wg.Done()
		for {
			select {
			case <-done:
				return
			case v, ok := <-input:
				if !ok {
					return
				}
				log.Printf("Полученные данные: %d\n", v)
			}
		}
	}()
}

func main() {
	done := make(chan struct{})
	defer close(done)

	// Создание источника данных
	consoleSource := &ConsoleSource{}
	intStream := consoleSource.Process(done)

	// Создание фильтров
	filterNegative := NewFilter(func(i int) bool {
		return i >= 0
	})
	filterMultiplesOfThree := NewFilter(func(i int) bool {
		return i%3 == 0 && i != 0
	})

	// Создание буфера
	ringBuffer := NewRingBuffer(bufferSize)

	fmt.Println("Введите числа для записи в буфер. Для завершени работы напишите 'exit'.")
	// Построение пайплайна
	processedData := ringBuffer.Process(
		filterMultiplesOfThree.Process(
			filterNegative.Process(intStream, done),
			done,
		),
		done,
	)

	// Создание потребителя
	consoleConsumer := &ConsoleConsumer{}
	consoleConsumer.Process(processedData, done)
	// Ожидание завершения потребителя
	consoleConsumer.wg.Wait()
}
