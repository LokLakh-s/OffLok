// Package i18n holds the words OffLok shows people, in English and French. Other languages are new maps.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

var catalogs = map[string]map[string]string{"en": en, "fr": fr}

// Lang returns the user's language among those OffLok speaks, English by default: the locale's variables, or
// the system's own setting where they are not set (Windows).
func Lang() string {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if l := os.Getenv(v); l != "" {
			return known(l)
		}
	}
	if l := systemLang(); l != "" {
		return known(l)
	}
	return "en"
}

// known reduces "fr_FR.UTF-8" or "fr-FR" to "fr" when OffLok speaks it, English otherwise.
func known(l string) string {
	if len(l) >= 2 && catalogs[strings.ToLower(l[:2])] != nil {
		return strings.ToLower(l[:2])
	}
	return "en"
}

// T returns the message for key in lang, formatted with args; English when lang lacks it.
func T(lang, key string, args ...any) string {
	msg, ok := catalogs[lang][key]
	if !ok {
		msg, ok = en[key]
	}
	if !ok {
		msg = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

var en = map[string]string{
	"colon":                   ":",
	"state.starting":          "Starting",
	"state.direct":            "On: resolving by itself, from the root servers",
	"state.fallback-dot":      "Fallback: the root servers are out of reach, using your DNS-over-TLS resolver %s",
	"state.fallback-network":  "Fallback: the root servers are out of reach, using this network's DNS (reduced protection)",
	"state.strict-blocked":    "Strict mode: the root servers are out of reach, public names do not resolve",
	"state.offline":           "No network: answers from the cache only",
	"state.off":               "Off: the system uses its original DNS",
	"state.error":             "Error: %s",
	"error.given-back":        "The system DNS was given back.",
	"error.strict-kept":       "Strict mode: the system DNS stays on OffLok.",
	"label.strict":            "Strict mode",
	"label.dot":               "DNS-over-TLS resolver",
	"label.access":            "Settings",
	"label.network":           "Network's DNS",
	"label.system":            "System",
	"value.on":                "on",
	"value.off":               "off",
	"value.none":              "none",
	"value.admins":            "administrators only",
	"value.everyone":          "everyone",
	"value.domains":           "domains",
	"err.denied":              "This needs administrator rights: run it again with sudo (or offlok --polkit, from a desktop).",
	"signin.until":            "The network's DNS is let through until %s, to sign in to this network.",
	"label.blocking":          "Blocking",
	"label.trusted":           "Trusted sites",
	"cat.ads":                 "ads",
	"cat.trackers":            "trackers",
	"cat.malicious":           "malicious sites",
	"cat.custom":              "your lists",
	"blocking.on":             "%s (%d names)",
	"blocking.failed":         ", %d list(s) failed to download",
	"blocking.paused":         ", paused until %s",
	"check.free":              "%s is not blocked.",
	"check.blocked":           "%s is blocked (%s: %s lists %s). To let it through: sudo offlok trust %[1]s",
	"check.trusted":           "%s is in your trusted sites (%s would block it).",
	"check.paused":            "%s is listed by %s, but the blocking is paused.",
	"tray.open":               "Open OffLok",
	"tray.quit":               "Quit the tray icon (OffLok keeps running)",
	"err.not-running":         "offlokd is not running. Start it with: sudo systemctl start offlokd",
	"err.denied.windows":      "This needs administrator rights: run it again from a terminal opened as administrator.",
	"err.not-running.windows": "The OffLok service is not running. Start it from Services, or: sc start OffLok (as administrator)",
	"done":                    "Done.",
	"label.updates":           "Updates",
	"updates.on":              "on (once a day, on GitHub)",
	"updates.last-error":      " (last check: %s)",
	"update.available":        "Update available: OffLok %s. To install it: sudo offlok update",
	"update.windows":          "Update available: OffLok %s. To install it: offlok update, from a terminal opened as administrator",
	"update.downloading":      "Update: downloading OffLok %s",
	"update.installing":       "Update: installing OffLok %s, which restarts it",
	"update.failed":           "Update: %s",
	"update.started":          "Downloading OffLok %s: it then installs itself and restarts.",
	"usage": `Usage: offlok [command]

  status                    what OffLok is doing (the default)
  on | off                  turn OffLok on, or off: off gives the system its original DNS back
  strict on | off           strict mode: never fall back to the network's DNS
  signin                    strict mode: let the network's DNS through for 5 minutes, to sign in to a Wi-Fi
  dot <name[:port]> | none  your DNS-over-TLS resolver, used when the root servers are out of reach
  access admins | everyone  who may change settings (the status stays open to everyone)
  version                   OffLok's version

  block <category> on|off   block ads, trackers, malicious or custom (your lists); all off at first
  check <site>              is this site blocked, and by which list?
  trust | untrust <site>    never block this site (and what is under it), or block it again
  pause [off]               let everything through for 10 minutes (open to everyone), or end the pause
  list add|remove <url>     your own lists (AdBlock, hosts or one domain per line), category custom

  updates on | off          look for a new version once a day on GitHub, which then sees this computer's address
  update                    download the new version, check its signature and install it

  --polkit before a command asks for the administrator's password in the desktop session, instead of sudo.
`,
}

var fr = map[string]string{
	"colon":                   " :", // French puts a space before the colon
	"state.starting":          "Démarrage",
	"state.direct":            "Actif : résout tout seul, depuis les serveurs racine",
	"state.fallback-dot":      "Repli : serveurs racine injoignables, passe par ton résolveur DNS-over-TLS %s",
	"state.fallback-network":  "Repli : serveurs racine injoignables, passe par le DNS de ce réseau (protection réduite)",
	"state.strict-blocked":    "Mode strict : serveurs racine injoignables, les noms publics ne répondent plus",
	"state.offline":           "Pas de réseau : réponses du cache seulement",
	"state.off":               "Éteint : le système utilise son DNS d'origine",
	"state.error":             "Erreur : %s",
	"error.given-back":        "Le DNS du système a été rendu.",
	"error.strict-kept":       "Mode strict : le DNS du système reste sur OffLok.",
	"label.strict":            "Mode strict",
	"label.dot":               "Résolveur DNS-over-TLS",
	"label.access":            "Réglages",
	"label.network":           "DNS du réseau",
	"label.system":            "Système",
	"value.on":                "oui",
	"value.off":               "non",
	"value.none":              "aucun",
	"value.admins":            "administrateurs seulement",
	"value.everyone":          "tout le monde",
	"value.domains":           "domaines",
	"err.denied":              "Il faut les droits d'administrateur : relance la commande avec sudo (ou offlok --polkit, depuis un bureau).",
	"signin.until":            "Le DNS du réseau passe jusqu'à %s, le temps de te connecter à ce réseau.",
	"label.blocking":          "Blocage",
	"label.trusted":           "Sites de confiance",
	"cat.ads":                 "pubs",
	"cat.trackers":            "pisteurs",
	"cat.malicious":           "sites piégés",
	"cat.custom":              "tes listes",
	"blocking.on":             "%s (%d noms)",
	"blocking.failed":         ", %d liste(s) pas téléchargée(s)",
	"blocking.paused":         ", en pause jusqu'à %s",
	"check.free":              "%s n'est pas bloqué.",
	"check.blocked":           "%s est bloqué (%s : %s contient %s). Pour le laisser passer : sudo offlok trust %[1]s",
	"check.trusted":           "%s fait partie de tes sites de confiance (%s le bloquerait).",
	"check.paused":            "%s est dans %s, mais le blocage est en pause.",
	"tray.open":               "Ouvrir OffLok",
	"tray.quit":               "Quitter l'icône (OffLok continue de tourner)",
	"err.not-running":         "offlokd ne tourne pas. Lance-le avec : sudo systemctl start offlokd",
	"err.denied.windows":      "Il faut les droits d'administrateur : relance la commande depuis un terminal ouvert en administrateur.",
	"err.not-running.windows": "Le service OffLok ne tourne pas. Lance-le depuis Services, ou : sc start OffLok (en administrateur)",
	"done":                    "C'est fait.",
	"label.updates":           "Mises à jour",
	"updates.on":              "oui (une fois par jour, sur GitHub)",
	"updates.last-error":      " (dernière vérification : %s)",
	"update.available":        "Mise à jour disponible : OffLok %s. Pour l'installer : sudo offlok update",
	"update.windows":          "Mise à jour disponible : OffLok %s. Pour l'installer : offlok update, depuis un terminal ouvert en administrateur",
	"update.downloading":      "Mise à jour : téléchargement d'OffLok %s",
	"update.installing":       "Mise à jour : installation d'OffLok %s, qui redémarre",
	"update.failed":           "Mise à jour : %s",
	"update.started":          "Téléchargement d'OffLok %s : il s'installe ensuite et redémarre.",
	"usage": `Utilisation : offlok [commande]

  status                    ce que fait OffLok (par défaut)
  on | off                  allumer OffLok, ou l'éteindre : le système retrouve son DNS d'origine
  strict on | off           mode strict : jamais de repli sur le DNS du réseau
  signin                    mode strict : laisser passer le DNS du réseau 5 minutes, le temps de se connecter à un Wi-Fi
  dot <nom[:port]> | none   ton résolveur DNS-over-TLS, utilisé quand les serveurs racine sont injoignables
  access admins | everyone  qui peut changer les réglages (l'état reste visible par tous)
  version                   la version d'OffLok

  block <catégorie> on|off  bloquer ads (pubs), trackers (pisteurs), malicious (sites piégés) ou custom (tes listes) ; tout est coupé au départ
  check <site>              ce site est-il bloqué, et par quelle liste ?
  trust | untrust <site>    ne jamais bloquer ce site (ni ce qui est dessous), ou le bloquer à nouveau
  pause [off]               tout laisser passer 10 minutes (ouvert à tous), ou finir la pause
  list add|remove <url>     tes propres listes (AdBlock, hosts ou un domaine par ligne), catégorie custom

  updates on | off          chercher une nouvelle version une fois par jour sur GitHub, qui voit alors l'adresse de cet ordinateur
  update                    télécharger la nouvelle version, vérifier sa signature et l'installer

  --polkit devant une commande demande le mot de passe administrateur dans la session du bureau, au lieu de sudo.
`,
}
