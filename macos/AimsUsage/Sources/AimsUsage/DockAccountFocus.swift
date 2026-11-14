import Foundation
import UsageCore

enum DockAccountFocus: Hashable, Identifiable {
    case all
    case account(String)

    init(savedValue: String?) {
        if let savedValue, savedValue.hasPrefix("account:"), savedValue.count > 8 {
            self = .account(String(savedValue.dropFirst(8)))
        } else {
            self = .all
        }
    }

    var id: String {
        switch self {
        case .all: "all"
        case .account(let id): "account:\(id)"
        }
    }

    static func supports(_ providerID: String) -> Bool { providerID == "claude" || providerID == "codex" }

    func canChoose(in provider: UsageProvider) -> Bool {
        Self.supports(provider.id) && (provider.rows.count > 1 || self != .all)
    }

    func includes(_ row: UsageRow) -> Bool {
        switch self {
        case .all: true
        case .account(let id): row.id == id
        }
    }

    func title(in provider: UsageProvider) -> String {
        switch self {
        case .all: return "All"
        case .account(let id):
            guard let row = provider.rows.first(where: { $0.id == id }) else { return "Missing" }
            return row.name.isEmpty ? row.id : ProviderStyle.accountName(row)
        }
    }

    static func options(for provider: UsageProvider, including selected: Self) -> [Self] {
        var seen = Set<String>()
        var choices: [Self] = [.all]
        for row in provider.rows where !row.id.isEmpty && seen.insert(row.id).inserted {
            choices.append(.account(row.id))
        }
        if !choices.contains(selected) { choices.append(selected) }
        return choices
    }
}
