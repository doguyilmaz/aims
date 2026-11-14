import AppKit
import UsageCore

enum DockNativeMenu {
    static func make(model: UsageModel, provider: UsageProvider?, openOverview: @escaping @MainActor () -> Void)
        -> NSMenu
    {
        let menu = NSMenu()
        menu.minimumWidth = 220
        if let provider {
            addFocusItems(to: menu, model: model, provider: provider)
        } else if let providers = model.snapshot?.displayProviders {
            let windows = NSMenu()
            for provider in providers {
                let item = NSMenuItem(title: ProviderStyle.title(provider), action: nil, keyEquivalent: "")
                let submenu = NSMenu()
                addFocusItems(to: submenu, model: model, provider: provider)
                item.submenu = submenu
                windows.addItem(item)
            }
            let item = NSMenuItem(title: "Usage windows", action: nil, keyEquivalent: "")
            item.submenu = windows
            menu.addItem(item)
        }
        menu.addItem(.separator())
        addHeading("Percentages", to: menu)
        for mode in DockValueMode.allCases {
            menu.addItem(
                DockMenuItem(title: mode.title, checked: mode == model.edgeDockValueMode) {
                    model.edgeDockValueMode = mode
                })
        }
        menu.addItem(.separator())
        menu.addItem(
            DockMenuItem(title: "Pin Edge Dock", checked: model.edgeDockPinned) { model.edgeDockPinned.toggle() })
        menu.addItem(DockMenuItem(title: "Open overview", perform: openOverview))
        menu.addItem(DockMenuItem(title: "Hide Edge Dock") { model.edgeDockEnabled = false })
        return menu
    }

    private static func addFocusItems(to menu: NSMenu, model: UsageModel, provider: UsageProvider) {
        addHeading("Usage window", to: menu)
        let selected = model.dockFocus(for: provider.id)
        let reported = DockWindowFocus.options(for: provider)
        for focus in DockWindowFocus.options(for: provider, including: selected) {
            let title = focus.title + (reported.contains(focus) ? "" : " · Not reported")
            menu.addItem(
                DockMenuItem(title: title, checked: focus == selected) { model.setDockFocus(focus, for: provider.id) })
        }
    }

    private static func addHeading(_ title: String, to menu: NSMenu) {
        let item = NSMenuItem(title: title, action: nil, keyEquivalent: "")
        item.isEnabled = false
        menu.addItem(item)
    }
}
