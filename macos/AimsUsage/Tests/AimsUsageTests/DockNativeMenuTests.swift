import AppKit
import Testing
import UsageCore

@testable import AimsUsage

@Test @MainActor func nativeDockMenuKeepsProviderChoicesAndActions() throws {
    _ = NSApplication.shared
    let keys = ["edgeDockWindowFocus", "edgeDockValueMode", "edgeDockPinned"]
    let saved = keys.map { UserDefaults.standard.object(forKey: $0) }
    defer {
        for (key, value) in zip(keys, saved) {
            if let value {
                UserDefaults.standard.set(value, forKey: key)
            } else {
                UserDefaults.standard.removeObject(forKey: key)
            }
        }
    }
    let snapshot = try UsageSnapshot.decode(
        Data(
            #"""
            {"schemaVersion":1,"generatedAt":"2026-11-14T00:00:00Z","providers":[
              {"id":"claude","name":"Claude","installed":true,"rows":[{"id":"a","name":"A","windows":[{"label":"5h","pct":10},{"label":"7d","pct":20}]}]},
              {"id":"codex","name":"Codex","installed":true,"rows":[]}
            ]}
            """#.utf8))
    let model = UsageModel()
    model.snapshot = snapshot
    model.setDockFocus(.automatic, for: "claude")
    model.setDockFocus(.window("month"), for: "codex")
    model.edgeDockValueMode = .used
    model.edgeDockPinned = false
    var openedOverview = false
    let menu = DockNativeMenu.make(model: model, provider: snapshot.providers[0]) { openedOverview = true }
    #expect(menu.size.width >= 220)
    #expect(menu.item(withTitle: "Automatic")?.state == .on)
    let weekly = try #require(menu.items.firstIndex { $0.title == "Weekly" })
    menu.performActionForItem(at: weekly)
    #expect(model.dockFocus(for: "claude") == .window("7d"))
    #expect(model.dockFocus(for: "codex") == .window("month"))
    menu.performActionForItem(at: try #require(menu.items.firstIndex { $0.title == "Remaining" }))
    #expect(model.edgeDockValueMode == .remaining)
    menu.performActionForItem(at: try #require(menu.items.firstIndex { $0.title == "Pin Edge Dock" }))
    #expect(model.edgeDockPinned)
    menu.performActionForItem(at: try #require(menu.items.firstIndex { $0.title == "Open overview" }))
    #expect(openedOverview)
    let overview = DockNativeMenu.make(model: model, provider: nil) {}
    #expect(overview.item(withTitle: "Usage windows")?.submenu?.items.count == 2)
}
