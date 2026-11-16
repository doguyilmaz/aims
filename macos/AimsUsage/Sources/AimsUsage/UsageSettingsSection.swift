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
}
