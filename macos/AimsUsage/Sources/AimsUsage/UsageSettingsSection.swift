import SwiftUI

enum UsageSettingsSection: String, CaseIterable, Identifiable {
    case general
    case dock
    case readings
    case cli
    case updates

    var id: Self { self }

    var title: String {
        switch self {
        case .general: "General"
        case .dock: "Layout & display"
        case .readings: "Usage readings"
        case .cli: "CLI & paths"
        case .updates: "Updates & About"
        }
    }

    var symbol: String {
        switch self {
        case .general: "slider.horizontal.3"
        case .dock: "sidebar.right"
        case .readings: "gauge.with.dots.needle.50percent"
        case .cli: "terminal"
        case .updates: "arrow.triangle.2.circlepath"
        }
    }

    var tint: Color {
        switch self {
        case .general: .blue
        case .dock: .purple
        case .readings: .orange
        case .cli: .gray
        case .updates: .green
        }
    }

    var subtitle: String {
        switch self {
        case .general: "Make usage feel at home on your Mac."
        case .dock: "Choose where your Dock lives and how it appears."
        case .readings: "Choose the percentages and limits shown in the Dock."
        case .cli: "Connect the app to your existing aims installation."
        case .updates: "Version, links and updates for Aims Usage."
        }
    }
}
