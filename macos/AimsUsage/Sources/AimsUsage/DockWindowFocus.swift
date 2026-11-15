import Foundation
import UsageCore

enum DockWindowFocus: Hashable, Identifiable {
    case automatic
    case window(String)

    init(savedValue: String?) {
        if let savedValue, savedValue.hasPrefix("window:"), savedValue.count > 7 {
            let label = Self.normalized(String(savedValue.dropFirst(7)))
            self = label.isEmpty ? .automatic : .window(label)
        } else {
            self = .automatic
        }
    }

    var id: String {
        switch self {
        case .automatic: "automatic"
        case .window(let label): "window:\(label)"
        }
    }

    var title: String {
        switch self {
        case .automatic: "Automatic"
        case .window("5h"): "5 hours"
        case .window("1d"): "Daily"
        case .window("7d"): "Weekly"
        case .window("month"): "Monthly"
        case .window("budget"): "Budget"
        case .window(let label): label
        }
    }

    func includes(_ window: UsageWindow) -> Bool {
        switch self {
        case .automatic: true
        case .window(let label): Self.normalized(window.label) == Self.normalized(label)
        }
    }

    static func options(for provider: UsageProvider, including selected: Self = .automatic) -> [Self] {
        let labels = Set(provider.rows.flatMap { $0.windows ?? [] }.map { normalized($0.label) }.filter { !$0.isEmpty })
        let rank = ["5h": 0, "1d": 1, "7d": 2, "month": 3, "budget": 4]
        var choices: [Self] =
            [.automatic]
            + labels.sorted {
                let first = rank[$0] ?? 5
                let second = rank[$1] ?? 5
                return first == second ? $0 < $1 : first < second
            }.map(Self.window)
        if !choices.contains(selected) { choices.append(selected) }
        return choices
    }

    private static func normalized(_ label: String) -> String {
        switch label.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
        case "weekly", "week", "7d": "7d"
        case "monthly", "month": "month"
        case "daily", "day", "1d": "1d"
        case "5 hours", "5-hour", "5h": "5h"
        default: label.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        }
    }
}
