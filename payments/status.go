package payments

import "slices"

// Status — этап, на котором находится платёж.
// Названия совпадают со статусами ЮKassa, кроме StatusNew: он существует только у нас.
type Status string

const (
	StatusNew       Status = "new"       // сохранён у нас, в ЮKassa ещё не отправлен
	StatusPending   Status = "pending"   // создан в ЮKassa, ждём, пока клиент оплатит
	StatusSucceeded Status = "succeeded" // деньги списаны
	StatusCanceled  Status = "canceled"  // платёж отменён или отклонён
)

// transitions — разрешённые переходы между статусами:
// ключ — текущий статус, значение — статусы, в которые из него можно перейти.
// StatusSucceeded и StatusCanceled здесь нет намеренно: это финальные статусы,
// для отсутствующего ключа map вернёт пустой список, то есть переходов нет.
var transitions = map[Status][]Status{
	StatusNew:     {StatusPending, StatusCanceled},
	StatusPending: {StatusSucceeded, StatusCanceled},
}

// CanTransitionTo сообщает, разрешён ли переход из статуса s в статус next.
func (s Status) CanTransitionTo(next Status) bool {
	return slices.Contains(transitions[s], next)
}

// IsFinal сообщает, что из статуса s переходов больше нет.
func (s Status) IsFinal() bool {
	return len(transitions[s]) == 0
}
