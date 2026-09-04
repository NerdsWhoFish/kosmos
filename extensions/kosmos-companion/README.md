# Kosmos Companion

Kosmos Companion is a Manifest V3 WebExtension for Chrome and Safari. It opens Google Voice and prepares a Kosmos contact's phone number for a call or message. You still review and start the communication yourself.

## Chrome

1. Download this folder.
2. Open `chrome://extensions`, enable Developer mode, and choose **Load unpacked**.
3. Select `extensions/kosmos-companion`.

## Safari

Safari Web Extensions use the same source through Apple's converter:

```sh
xcrun safari-web-extension-converter extensions/kosmos-companion
```

Build and enable the generated app extension in Xcode. Normal Safari distribution requires Apple signing and App Store review.

Google Voice does not publish a supported message-composer deep link. The extension identifies Voice's phone-number field by its accessible label, so a future Voice UI change may require updating `voice.js`.
