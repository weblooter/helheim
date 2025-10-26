package main

import (
	"helheim/internal/entity"
	"helheim/internal/service/scanner"
	"log"
	"time"
)

func main() {

	scan := scanner.NewScanner("/home/weblooter/Storage/work/helheim/src")
	scan.SetBaseState(make(entity.FilesStruct)) // TODO получить изменения с recipient и положить их как базовое состояние

	for {
		log.Printf("== New scan")

		// Запуск сканирования
		err := scan.Rescan()
		if err != nil {
			log.Fatal(err)
		}

		// Получим изменения
		changes := scan.GetChanges()
		for action, files := range changes {
			if len(files) > 0 {
				log.Printf("action: %s, files: %v", action, len(files))

				// TODO Отправить запрос на совершение действия получателю
				//for filepathHash, file := range files {
				for filepathHash, _ := range files {
					err = scan.CommitFile(action, filepathHash)
					if err != nil {
						log.Fatal(err)
					}
				}
			}
		}

		time.Sleep(5 * time.Second)
	}
}
