package uploadscan

import (
	"strconv"
	"strings"
)

// messages are the texts of the answer to a refused upload, by language and reason (refuse.go):
// the plugin's message file, a table since Yaegi loads the standard library only. {detail} is
// what the refusal names (the signature, the size limit), {seconds} its Retry-After. German in
// Swiss spelling.
var messages = map[string]map[string]string{
	"en": {
		"title":           "Upload refused",
		"retry":           "Please try again in {seconds} seconds.",
		"infected":        "The file contains malware ({detail}) and was not accepted.",
		"limits-exceeded": "The file goes beyond what the malware check can examine ({detail}) and was not accepted.",
		"ambiguous":       "The upload is put together in an unusual way and could not be checked reliably. Please upload the file again with a current browser or app.",
		"too-large":       "The upload is too large to be checked for malware (at most {detail} MB).",
		"busy":            "Too many uploads are being checked right now.",
		"too-many":        "Too many uploads from your network are being checked at once.",
		"scan-time":       "Checking the upload for malware took too long.",
		"unavailable":     "The malware check is not available right now.",
		"incomplete-body": "The upload did not arrive in full. Please try again.",
	},
	"de": {
		"title":           "Upload abgelehnt",
		"retry":           "Bitte versuchen Sie es in {seconds} Sekunden noch einmal.",
		"infected":        "Die Datei enthält Schadsoftware ({detail}) und wurde nicht angenommen.",
		"limits-exceeded": "Die Datei überschreitet, was die Prüfung auf Schadsoftware untersuchen kann ({detail}), und wurde nicht angenommen.",
		"ambiguous":       "Der Upload ist ungewöhnlich aufgebaut und liess sich nicht zuverlässig prüfen. Bitte laden Sie die Datei mit einem aktuellen Browser oder einer aktuellen App erneut hoch.",
		"too-large":       "Der Upload ist zu gross, um auf Schadsoftware geprüft zu werden (höchstens {detail} MB).",
		"busy":            "Gerade werden zu viele Uploads geprüft.",
		"too-many":        "Aus Ihrem Netzwerk werden gerade zu viele Uploads gleichzeitig geprüft.",
		"scan-time":       "Die Prüfung des Uploads auf Schadsoftware hat zu lange gedauert.",
		"unavailable":     "Die Prüfung auf Schadsoftware ist gerade nicht verfügbar.",
		"incomplete-body": "Der Upload ist nicht vollständig angekommen. Bitte versuchen Sie es noch einmal.",
	},
	"fr": {
		"title":           "Envoi refusé",
		"retry":           "Veuillez réessayer dans {seconds} secondes.",
		"infected":        "Le fichier contient un logiciel malveillant ({detail}) et n'a pas été accepté.",
		"limits-exceeded": "Le fichier dépasse ce que l'analyse antivirus peut examiner ({detail}) et n'a pas été accepté.",
		"ambiguous":       "L'envoi a une structure inhabituelle et n'a pas pu être vérifié de manière fiable. Veuillez renvoyer le fichier avec un navigateur ou une application à jour.",
		"too-large":       "L'envoi est trop volumineux pour être analysé ({detail} Mo au maximum).",
		"busy":            "Trop d'envois sont en cours d'analyse en ce moment.",
		"too-many":        "Trop d'envois provenant de votre réseau sont analysés en même temps.",
		"scan-time":       "L'analyse antivirus de l'envoi a pris trop de temps.",
		"unavailable":     "L'analyse antivirus n'est pas disponible pour le moment.",
		"incomplete-body": "L'envoi n'est pas arrivé en entier. Veuillez réessayer.",
	},
	"it": {
		"title":           "Caricamento rifiutato",
		"retry":           "Riprova tra {seconds} secondi.",
		"infected":        "Il file contiene malware ({detail}) e non è stato accettato.",
		"limits-exceeded": "Il file supera ciò che il controllo antivirus può esaminare ({detail}) e non è stato accettato.",
		"ambiguous":       "Il caricamento ha una struttura insolita e non è stato possibile controllarlo in modo affidabile. Carica di nuovo il file con un browser o un'app aggiornati.",
		"too-large":       "Il caricamento è troppo grande per essere controllato (al massimo {detail} MB).",
		"busy":            "In questo momento si stanno controllando troppi caricamenti.",
		"too-many":        "Troppi caricamenti dalla tua rete vengono controllati contemporaneamente.",
		"scan-time":       "Il controllo antivirus del caricamento ha richiesto troppo tempo.",
		"unavailable":     "Il controllo antivirus non è disponibile al momento.",
		"incomplete-body": "Il caricamento non è arrivato per intero. Riprova.",
	},
	"es": {
		"title":           "Subida rechazada",
		"retry":           "Vuelve a intentarlo dentro de {seconds} segundos.",
		"infected":        "El archivo contiene malware ({detail}) y no se ha aceptado.",
		"limits-exceeded": "El archivo supera lo que el análisis antivirus puede examinar ({detail}) y no se ha aceptado.",
		"ambiguous":       "La subida tiene una estructura inusual y no se ha podido comprobar de forma fiable. Vuelve a subir el archivo con un navegador o una aplicación actualizados.",
		"too-large":       "La subida es demasiado grande para analizarla (como máximo {detail} MB).",
		"busy":            "Ahora mismo se están comprobando demasiadas subidas.",
		"too-many":        "Se están comprobando demasiadas subidas de tu red a la vez.",
		"scan-time":       "El análisis antivirus de la subida ha tardado demasiado.",
		"unavailable":     "El análisis antivirus no está disponible en este momento.",
		"incomplete-body": "La subida no ha llegado completa. Vuelve a intentarlo.",
	},
}

func (r *refusal) text(lang string) string {
	return strings.ReplaceAll(messages[lang][r.reason], "{detail}", r.detail)
}

func (r *refusal) retry(lang string) string {
	if r.retryAfter == 0 {
		return ""
	}
	return strings.ReplaceAll(messages[lang]["retry"], "{seconds}", strconv.Itoa(r.retryAfter))
}
