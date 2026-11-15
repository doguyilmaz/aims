import Foundation

public struct UsageWindow: Decodable, Sendable {
    public let id: String?
    public let label: String
    public let pct: Double
    public let pctUnknown: Bool?
    public let limited: Bool?
    public let resetsAt: Date?

    public var validPercent: Double? { pctUnknown != true && pct.isFinite && pct >= 0 ? pct : nil }
    public func needsRefresh(at now: Date) -> Bool { resetsAt.map { $0 <= now } ?? false }
}
