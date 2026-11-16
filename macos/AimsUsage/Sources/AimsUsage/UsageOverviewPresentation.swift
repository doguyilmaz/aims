/// The native close callback completes a source change before the same popover is shown again.
enum UsageOverviewPresentation: Equatable {
    case hidden
    case presented(UsageOverviewRequest)
    case closing(current: UsageOverviewRequest, next: UsageOverviewRequest?)

    var request: UsageOverviewRequest? {
        switch self {
        case .hidden: nil
        case .presented(let request), .closing(let request, _): request
        }
    }

    var isVisible: Bool { request != nil }
    var isPinned: Bool { request?.pinned == true }
    var holdsDockOpen: Bool { request?.source == .dock }

    mutating func toggle(from source: UsageOverviewSource, pinnedWhenOpening: Bool = false) {
        switch self {
        case .hidden:
            self = .presented(UsageOverviewRequest(source: source, pinned: pinnedWhenOpening))
        case .presented(let current), .closing(let current, _):
            let next = source == current.source ? nil : UsageOverviewRequest(source: source, pinned: current.pinned)
            self = .closing(current: current, next: next)
        }
    }

    mutating func togglePin() {
        guard case .presented(var request) = self else { return }
        request.pinned.toggle()
        self = .presented(request)
    }

    mutating func show(from source: UsageOverviewSource, pinnedWhenOpening: Bool = false) {
        switch self {
        case .hidden:
            self = .presented(UsageOverviewRequest(source: source, pinned: pinnedWhenOpening))
        case .presented(let current):
            if current.source != source {
                self = .closing(current: current, next: UsageOverviewRequest(source: source, pinned: current.pinned))
            }
        case .closing(let current, let next):
            self = .closing(
                current: current, next: UsageOverviewRequest(source: source, pinned: next?.pinned ?? current.pinned))
        }
    }

    mutating func close() {
        guard let current = request else { return }
        self = .closing(current: current, next: nil)
    }

    mutating func willClose() {
        if case .presented(let current) = self { self = .closing(current: current, next: nil) }
    }

    mutating func didClose() {
        if case .closing(_, let next?) = self {
            self = .presented(next)
        } else {
            self = .hidden
        }
    }
}
