package payments

import "slices"

// Status — этап, на котором находится платёж.
type Status string

const (
	StatusNew     Status = "new"     // создан, в банк ещё не отправлен
	StatusWaiting Status = "waiting" // отправлен в банк, ждём ответа
	StatusSuccess Status = "success" // банк подтвердил списание
	StatusDenied  Status = "denied"  // банк отклонил платёж
)

// transitions — разрешённые переходы между статусами:
// ключ — текущий статус, значение — статусы, в которые из него можно перейти.
// StatusSuccess и StatusDenied здесь нет намеренно: это финальные статусы,
// для отсутствующего ключа map вернёт пустой список, то есть переходов нет.
var transitions = map[Status][]Status{
	StatusNew:     {StatusWaiting},
	StatusWaiting: {StatusSuccess, StatusDenied},
}

// CanTransitionTo сообщает, разрешён ли переход из статуса s в статус next.
func (s Status) CanTransitionTo(next Status) bool {
	return slices.Contains(transitions[s], next)
}

// IsFinal сообщает, что из статуса s переходов больше нет.
func (s Status) IsFinal() bool {
	return len(transitions[s]) == 0
}
