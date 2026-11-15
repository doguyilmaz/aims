enum UsageControlIcon {
    case close, pin, pinned, refresh, settings, overview, more, usage, info, warning, bankedReset, trash

    var symbol: String {
        switch self {
        case .close: "xmark"
        case .pin: "pin"
        case .pinned: "pin.fill"
        case .refresh: "arrow.clockwise"
        case .settings: "gearshape"
        case .overview: "rectangle.grid.1x2"
        case .more: "ellipsis"
        case .usage: "chart.bar"
        case .info: "info.circle"
        case .warning: "exclamationmark.circle"
        case .bankedReset: "arrow.counterclockwise.circle"
        case .trash: "trash"
        }
    }
}
