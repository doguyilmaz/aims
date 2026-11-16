import SwiftUI

struct SettingsWindowStyle: NSViewRepresentable {
    func makeNSView(context: Context) -> SettingsWindowView { SettingsWindowView() }
    func updateNSView(_ nsView: SettingsWindowView, context: Context) {}
}
