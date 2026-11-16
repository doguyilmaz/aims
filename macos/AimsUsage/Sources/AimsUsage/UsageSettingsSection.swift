enum UsageSettingsSection: String, CaseIterable, Identifiable {
    case general
    case dock
    case cli
    case updates

    var id: Self { self }

    var title: String {
        switch self {
        case .general: "General"
        case .dock: "Edge Dock"
        case .cli: "CLI & paths"
        case .updates: "Updates"
        }
    }

    var symbol: String {
        switch self {
        case .general: "slider.horizontal.3"
        case .dock: "sidebar.right"
        case .cli: "terminal"
        case .updates: "arrow.triangle.2.circlepath"
        }
    }

    var subtitle: String {
        switch self {
        case .general: "Appearance and refresh for your usage overview."
        case .dock: "Choose a display and make the Dock yours."
        case .cli: "Connect the app to your existing aims setup."
        case .updates: "Keep the menu app, Edge Dock and bundled CLI up to date."
        }
    }
}
