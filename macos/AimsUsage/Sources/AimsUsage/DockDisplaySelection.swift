enum DockDisplaySelection: Hashable, Identifiable {
    case primary
    case display(String)

    init(savedValue: String?) {
        if let savedValue, savedValue.hasPrefix("display:"), savedValue.count > 8 {
            self = .display(String(savedValue.dropFirst(8)))
        } else {
            self = .primary
        }
    }

    var id: String {
        switch self {
        case .primary: "primary"
        case .display(let id): "display:\(id)"
        }
    }

    func isConnected(in displays: [DockDisplay]) -> Bool {
        switch self {
        case .primary: !displays.isEmpty
        case .display(let id): displays.contains { $0.id == id }
        }
    }

    func resolve(in displays: [DockDisplay]) -> DockDisplay? {
        if case .display(let id) = self, let selected = displays.first(where: { $0.id == id }) { return selected }
        // The first current NSScreen is the primary display. NSScreen.main follows
        // keyboard focus and would let Settings or the overview move the Dock.
        return displays.first
    }
}
