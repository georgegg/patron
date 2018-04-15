package amqp

import (
	"context"

	"github.com/mantzas/patron"
	"github.com/mantzas/patron/log"
	"github.com/pkg/errors"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

// Processor implementation of a RabbitMQ client
type Processor struct {
	url   string
	queue string
	mp    patron.MessageProcessor
}

// New returns a new client
func New(url, queue string, mp patron.MessageProcessor) (*Processor, error) {

	if url == "" {
		return nil, errors.New("rabbitmq url is required")
	}

	if queue == "" {
		return nil, errors.New("rabbitmq queue name is required")
	}

	if mp == nil {
		return nil, errors.New("work processor is required")
	}

	return &Processor{url, queue, mp}, nil
}

// Process items of the queue
func (p Processor) Process(ctx context.Context) error {

	conn, err := amqp.Dial(p.url)
	if err != nil {
		return errors.Wrapf(err, "failed to dial @ %s", p.url)
	}

	ch, err := conn.Channel()
	if err != nil {
		return errors.Wrap(err, "failed get channel")
	}

	tag := uuid.New().String()
	log.Infof("consuming messages for tag %s", tag)

	deliveries, err := ch.Consume(p.queue, tag, false, false, false, false, nil)
	if err != nil {
		return errors.Wrap(err, "failed initialize consumer")
	}

	procFailed := false

	select {
	case <-ctx.Done():
		log.Info("canceling requested")
		break
	case d := <-deliveries:
		log.Infof("processing message %s", d.MessageId)

		go func(d *amqp.Delivery, failed *bool) {

			err := p.mp.Process(ctx, d.Body)
			if err != nil {
				log.Errorf("failed to process message %s with %v", d.MessageId, err)
				procFailed = true
				return
			}
			d.Ack(false)
		}(&d, &procFailed)

		if procFailed {
			break
		}
	}

	err = ch.Cancel(tag, true)
	if err != nil {
		log.Errorf("failed to cancel channel of consumer %s", tag)
	}

	return errors.Wrap(conn.Close(), "failed to close connection")
}