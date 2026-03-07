package telegram

// Lang holds all user-facing strings for a given language.
type Lang struct {
	// ── General ─────────────────────────────────────────────────────
	BotOnline       string
	UnknownCommand  string
	PromptCancelled string
	OrCancel        string
	ItemNotFound    string

	// ── Command Menu Descriptions (Telegram /setMyCommands) ────────
	CmdDescStart       string
	CmdDescHelp        string
	CmdDescList        string
	CmdDescAdd         string
	CmdDescCheck       string
	CmdDescStatus      string
	CmdDescSetInterval string
	CmdDescSmart       string
	CmdDescLang        string
	CmdDescCancel      string

	// ── Help ────────────────────────────────────────────────────────
	HelpTitle       string
	HelpItemMgmt    string
	HelpList        string
	HelpAdd         string
	HelpMonitoring  string
	HelpCheck       string
	HelpStatus      string
	HelpSettings    string
	HelpSetInterval string
	HelpSmart       string
	HelpOther       string
	HelpCancel      string
	HelpHelp        string

	// ── List / Items ────────────────────────────────────────────────
	NoItemsYet    string
	TrackedItems  string
	TapToManage   string
	ItemDetail    string // "Item #%d"
	Target        string
	Status        string
	Active        string
	Paused        string
	LastCheck     string
	Never         string
	BackToList    string

	// ── Item Actions ────────────────────────────────────────────────
	BtnEdit   string
	BtnCheck  string
	BtnPause  string
	BtnResume string
	BtnRemove string
	BtnBack   string
	BtnURL    string
	BtnPrice  string

	// ── Add Flow ────────────────────────────────────────────────────
	AddPromptURL   string
	AddPromptPrice string
	AddSuccess     string // "Item #%d added!"
	AddFailed      string
	InvalidURL     string
	InvalidPrice   string

	// ── Edit Flow ───────────────────────────────────────────────────
	EditTitle     string // "Edit Item #%d"
	EditWhatChange string
	EditURLPrompt  string // "Send me the new Amazon URL\nCurrent: %s"
	EditPricePrompt string // "Send me the new target price\nCurrent: %.2f EUR"
	URLUpdated     string
	PriceUpdated   string // "Target price updated to %.2f EUR."
	SaveFailed     string

	// ── Remove Flow ─────────────────────────────────────────────────
	RemoveConfirm  string // "Remove Item #%d?"
	RemoveWarning  string
	RemoveYes      string
	RemoveNo       string
	RemoveSuccess  string // "Item #%d (%s) removed."
	RemoveFailed   string

	// ── Check ───────────────────────────────────────────────────────
	CheckingAll    string
	CheckingItem   string // "Checking %s..."
	CheckResult    string
	AutoCheckResult string
	AvailableNew   string // "%s available (NEW) at %.2f EUR"
	OutOfStockUsed string // "%s out of stock (new)\nUsed offer: %.2f EUR"
	OutOfStock     string // "%s out of stock. No offers found."
	NoItemsToCheck string

	// ── Alert ───────────────────────────────────────────────────────
	AlertTitle string
	AlertItem  string
	AlertPrice string
	AlertBuy   string

	// ── Status ──────────────────────────────────────────────────────
	GlobalStatus   string
	StatusItems    string // "%d total (%d active)"
	StatusInterval string
	StatusSmart    string

	// ── Settings ────────────────────────────────────────────────────
	IntervalPrompt  string // "Send me the new check interval\nCurrent: %.0fs"
	IntervalUpdated string // "Check interval updated to %d seconds."
	IntervalFailed  string
	InvalidInterval string
	SmartPrompt     string
	SmartEnabled    string
	SmartDisabled   string
	SmartFailed     string
	InvalidSmartVal string

	// ── Errors (from main loop via bot) ─────────────────────────────
	BlockedMsg   string // "Blocked! (HTTP %d) on #%d (%s). Cooling off %v."
	CheckError   string // "Error checking #%d (%s): %v"

	// ── Language ────────────────────────────────────────────────────
	LangPrompt   string
	LangChanged  string // "Language changed to %s."
	HelpLang     string
	StatusLang   string
}

var langEN = Lang{
	BotOnline:       "🤖 <b>Amazon Sniper Bot online!</b>\nType /help to get started.",
	UnknownCommand:  "❓ Unknown command. Type <code>/help</code>.",
	PromptCancelled: "🚫 Prompt cancelled.",
	OrCancel:        "<i>(Or type /cancel)</i>",
	ItemNotFound:    "❌ Item not found.",

	CmdDescStart:       "Show welcome message",
	CmdDescHelp:        "Show available commands",
	CmdDescList:        "Show all tracked items",
	CmdDescAdd:         "Add a new item to track",
	CmdDescCheck:       "Force check all enabled items",
	CmdDescStatus:      "Show global settings",
	CmdDescSetInterval: "Set poll interval in seconds",
	CmdDescSmart:       "Toggle smart notifications",
	CmdDescLang:        "Change language",
	CmdDescCancel:      "Cancel current prompt",

	HelpTitle:       "🤖 <b>Amazon Sniper Bot</b>",
	HelpItemMgmt:    "<b>Item Management</b>",
	HelpList:        "/list - Show all tracked items (tap to manage)",
	HelpAdd:         "/add - Add a new item to track",
	HelpMonitoring:  "<b>Monitoring</b>",
	HelpCheck:       "/check - Force check all enabled items",
	HelpStatus:      "/status - Show global settings",
	HelpSettings:    "<b>Settings</b>",
	HelpSetInterval: "/setinterval - Set poll interval (seconds)",
	HelpSmart:       "/smart - Toggle smart notifications (on/off)",
	HelpLang:        "/lang - Change language",
	HelpOther:       "<b>Other</b>",
	HelpCancel:      "/cancel - Cancel current prompt",
	HelpHelp:        "/help - Show this message",

	NoItemsYet:  "📭 No items tracked yet.\nUse /add to add one!",
	TrackedItems: "📋 <b>Tracked Items</b>",
	TapToManage:  "Tap an item to manage it:",
	ItemDetail:   "📦 <b>Item #%d</b>",
	Target:       "Target",
	Status:       "Status",
	Active:       "▶️ Active",
	Paused:       "⏸ Paused",
	LastCheck:    "Last check",
	Never:        "Never",
	BackToList:   "🔙 Back to list",

	BtnEdit:   "✏️ Edit",
	BtnCheck:  "🔍 Check",
	BtnPause:  "⏸ Pause",
	BtnResume: "▶️ Resume",
	BtnRemove: "🗑 Remove",
	BtnBack:   "🔙 Back",
	BtnURL:    "🔗 URL",
	BtnPrice:  "💰 Price",

	AddPromptURL:   "🔗 <b>Send me the Amazon product URL:</b>",
	AddPromptPrice: "💰 <b>Now send me the target price (e.g. 900.00):</b>",
	AddSuccess:     "✅ Item #%d added!\n📦 %s\n💰 Target: %.2f EUR",
	AddFailed:      "⚠️ Failed to add item: %v",
	InvalidURL:     "❌ That doesn't look like an Amazon URL. Try again or /cancel",
	InvalidPrice:   "❌ Invalid price. Try again (e.g. 850.00) or /cancel",

	EditTitle:       "✏️ <b>Edit Item #%d</b> (%s)",
	EditWhatChange:  "What do you want to change?",
	EditURLPrompt:   "🔗 <b>Send me the new Amazon URL</b>\nCurrent: %s",
	EditPricePrompt: "💰 <b>Send me the new target price</b>\nCurrent: <b>%.2f EUR</b>",
	URLUpdated:      "✅ URL updated.",
	PriceUpdated:    "✅ Target price updated to %.2f EUR.",
	SaveFailed:      "⚠️ Failed to save: %v",

	RemoveConfirm: "⚠️ <b>Remove Item #%d?</b>",
	RemoveWarning: "This cannot be undone.",
	RemoveYes:     "✅ Yes, remove",
	RemoveNo:      "❌ Cancel",
	RemoveSuccess: "🗑 Item #%d (%s) removed.",
	RemoveFailed:  "⚠️ Failed to remove: %v",

	CheckingAll:    "🔍 <b>Forcing check on all enabled items...</b>",
	CheckingItem:   "🔍 <b>Checking %s...</b>",
	CheckResult:    "🔍 <b>Check Result</b>",
	AutoCheckResult: "🔄 <b>Auto-check Result</b>",
	AvailableNew:   "%s %s available (NEW) at <b>%.2f EUR</b>\n🎯 Target: &lt; %.2f EUR",
	OutOfStockUsed: "❌ %s out of stock (new)\n📦 Used offer: <b>%.2f EUR</b>",
	OutOfStock:     "❌ %s out of stock. No offers found.",
	NoItemsToCheck: "📭 No items to check. Use /add to add items.",

	AlertTitle: "🚨 <b>AMAZON SNIPER ALERT!</b>",
	AlertItem:  "📦 Item: <b>%s</b> (#%d)",
	AlertPrice: "💰 Price: <b>%.2f EUR</b>",
	AlertBuy:   "🔗 <a href=\"%s\">Buy on Amazon</a>",

	GlobalStatus:   "📊 <b>GLOBAL STATUS</b>",
	StatusItems:    "📦 Items: <b>%d total</b> (%d active)",
	StatusInterval: "⏱ Interval: <b>%.0fs</b>",
	StatusSmart:    "🧠 Smart Notify: <b>%s</b>",

	IntervalPrompt:  "⏱ <b>Send me the new check interval in seconds</b>\nCurrent: <b>%.0fs</b>",
	IntervalUpdated: "✅ Check interval updated to %d seconds.",
	IntervalFailed:  "⚠️ Interval updated to %ds, but failed to save.",
	InvalidInterval: "❌ Invalid interval. Try again (e.g. 60) or /cancel",
	SmartPrompt:     "🧠 <b>Enable Smart Notifications? (on/off):</b>",
	SmartEnabled:    "✅ Smart Notifications enabled.",
	SmartDisabled:   "✅ Smart Notifications disabled.",
	SmartFailed:     "⚠️ Smart Notifications changed (not saved).",
	InvalidSmartVal: "❌ Please reply with <code>on</code> or <code>off</code>, or /cancel",

	BlockedMsg: "⚠️ <b>Blocked!</b> (HTTP %d) on #%d (%s). Cooling off %v.",
	CheckError: "❌ Error checking #%d (%s): %v",

	LangPrompt:  "🌐 <b>Select language:</b>",
	LangChanged: "✅ Language changed to <b>%s</b>.",
	StatusLang:  "🌐 Language: <b>%s</b>",
}

var langIT = Lang{
	BotOnline:       "🤖 <b>Amazon Sniper Bot online!</b>\nDigita /help per iniziare.",
	UnknownCommand:  "❓ Comando sconosciuto. Digita <code>/help</code>.",
	PromptCancelled: "🚫 Azione annullata.",
	OrCancel:        "<i>(Oppure digita /cancel)</i>",
	ItemNotFound:    "❌ Articolo non trovato.",

	CmdDescStart:       "Mostra messaggio di benvenuto",
	CmdDescHelp:        "Mostra i comandi disponibili",
	CmdDescList:        "Mostra tutti gli articoli monitorati",
	CmdDescAdd:         "Aggiungi un nuovo articolo",
	CmdDescCheck:       "Controlla subito tutti gli articoli",
	CmdDescStatus:      "Mostra impostazioni globali",
	CmdDescSetInterval: "Imposta intervallo di controllo",
	CmdDescSmart:       "Attiva/disattiva notifiche intelligenti",
	CmdDescLang:        "Cambia lingua",
	CmdDescCancel:      "Annulla il prompt corrente",

	HelpTitle:       "🤖 <b>Amazon Sniper Bot</b>",
	HelpItemMgmt:    "<b>Gestione Articoli</b>",
	HelpList:        "/list - Mostra tutti gli articoli (tocca per gestire)",
	HelpAdd:         "/add - Aggiungi un nuovo articolo",
	HelpMonitoring:  "<b>Monitoraggio</b>",
	HelpCheck:       "/check - Controlla subito tutti gli articoli attivi",
	HelpStatus:      "/status - Mostra impostazioni globali",
	HelpSettings:    "<b>Impostazioni</b>",
	HelpSetInterval: "/setinterval - Imposta intervallo di controllo (secondi)",
	HelpSmart:       "/smart - Attiva/disattiva notifiche intelligenti",
	HelpLang:        "/lang - Cambia lingua",
	HelpOther:       "<b>Altro</b>",
	HelpCancel:      "/cancel - Annulla il prompt corrente",
	HelpHelp:        "/help - Mostra questo messaggio",

	NoItemsYet:  "📭 Nessun articolo monitorato.\nUsa /add per aggiungerne uno!",
	TrackedItems: "📋 <b>Articoli Monitorati</b>",
	TapToManage:  "Tocca un articolo per gestirlo:",
	ItemDetail:   "📦 <b>Articolo #%d</b>",
	Target:       "Obiettivo",
	Status:       "Stato",
	Active:       "▶️ Attivo",
	Paused:       "⏸ In pausa",
	LastCheck:    "Ultimo controllo",
	Never:        "Mai",
	BackToList:   "🔙 Torna alla lista",

	BtnEdit:   "✏️ Modifica",
	BtnCheck:  "🔍 Controlla",
	BtnPause:  "⏸ Pausa",
	BtnResume: "▶️ Riprendi",
	BtnRemove: "🗑 Rimuovi",
	BtnBack:   "🔙 Indietro",
	BtnURL:    "🔗 URL",
	BtnPrice:  "💰 Prezzo",

	AddPromptURL:   "🔗 <b>Inviami l'URL del prodotto Amazon:</b>",
	AddPromptPrice: "💰 <b>Ora inviami il prezzo obiettivo (es. 900.00):</b>",
	AddSuccess:     "✅ Articolo #%d aggiunto!\n📦 %s\n💰 Obiettivo: %.2f EUR",
	AddFailed:      "⚠️ Impossibile aggiungere l'articolo: %v",
	InvalidURL:     "❌ Non sembra un URL Amazon. Riprova o /cancel",
	InvalidPrice:   "❌ Prezzo non valido. Riprova (es. 850.00) o /cancel",

	EditTitle:       "✏️ <b>Modifica Articolo #%d</b> (%s)",
	EditWhatChange:  "Cosa vuoi modificare?",
	EditURLPrompt:   "🔗 <b>Inviami il nuovo URL Amazon</b>\nAttuale: %s",
	EditPricePrompt: "💰 <b>Inviami il nuovo prezzo obiettivo</b>\nAttuale: <b>%.2f EUR</b>",
	URLUpdated:      "✅ URL aggiornato.",
	PriceUpdated:    "✅ Prezzo obiettivo aggiornato a %.2f EUR.",
	SaveFailed:      "⚠️ Salvataggio fallito: %v",

	RemoveConfirm: "⚠️ <b>Rimuovere Articolo #%d?</b>",
	RemoveWarning: "Questa azione è irreversibile.",
	RemoveYes:     "✅ Sì, rimuovi",
	RemoveNo:      "❌ Annulla",
	RemoveSuccess: "🗑 Articolo #%d (%s) rimosso.",
	RemoveFailed:  "⚠️ Impossibile rimuovere: %v",

	CheckingAll:    "🔍 <b>Controllo forzato su tutti gli articoli attivi...</b>",
	CheckingItem:   "🔍 <b>Controllo %s...</b>",
	CheckResult:    "🔍 <b>Risultato Controllo</b>",
	AutoCheckResult: "🔄 <b>Risultato Auto-controllo</b>",
	AvailableNew:   "%s %s disponibile (NUOVO) a <b>%.2f EUR</b>\n🎯 Obiettivo: &lt; %.2f EUR",
	OutOfStockUsed: "❌ %s esaurito (nuovo)\n📦 Offerta usato: <b>%.2f EUR</b>",
	OutOfStock:     "❌ %s esaurito. Nessuna offerta trovata.",
	NoItemsToCheck: "📭 Nessun articolo da controllare. Usa /add per aggiungerne.",

	AlertTitle: "🚨 <b>AMAZON SNIPER ALERT!</b>",
	AlertItem:  "📦 Articolo: <b>%s</b> (#%d)",
	AlertPrice: "💰 Prezzo: <b>%.2f EUR</b>",
	AlertBuy:   "🔗 <a href=\"%s\">Acquista su Amazon</a>",

	GlobalStatus:   "📊 <b>STATO GLOBALE</b>",
	StatusItems:    "📦 Articoli: <b>%d totali</b> (%d attivi)",
	StatusInterval: "⏱ Intervallo: <b>%.0fs</b>",
	StatusSmart:    "🧠 Notifiche Smart: <b>%s</b>",

	IntervalPrompt:  "⏱ <b>Inviami il nuovo intervallo in secondi</b>\nAttuale: <b>%.0fs</b>",
	IntervalUpdated: "✅ Intervallo aggiornato a %d secondi.",
	IntervalFailed:  "⚠️ Intervallo aggiornato a %ds, ma salvataggio fallito.",
	InvalidInterval: "❌ Intervallo non valido. Riprova (es. 60) o /cancel",
	SmartPrompt:     "🧠 <b>Attivare le Notifiche Intelligenti? (on/off):</b>",
	SmartEnabled:    "✅ Notifiche Intelligenti attivate.",
	SmartDisabled:   "✅ Notifiche Intelligenti disattivate.",
	SmartFailed:     "⚠️ Notifiche Intelligenti cambiate (non salvate).",
	InvalidSmartVal: "❌ Rispondi con <code>on</code> o <code>off</code>, oppure /cancel",

	BlockedMsg: "⚠️ <b>Bloccato!</b> (HTTP %d) su #%d (%s). Raffreddamento %v.",
	CheckError: "❌ Errore controllo #%d (%s): %v",

	LangPrompt:  "🌐 <b>Seleziona la lingua:</b>",
	LangChanged: "✅ Lingua cambiata in <b>%s</b>.",
	StatusLang:  "🌐 Lingua: <b>%s</b>",
}

// LangMap maps language codes to their string tables.
var LangMap = map[string]*Lang{
	"en": &langEN,
	"it": &langIT,
}

// GetLang returns the Lang for the given code, defaulting to English.
func GetLang(code string) *Lang {
	if l, ok := LangMap[code]; ok {
		return l
	}
	return &langEN
}
