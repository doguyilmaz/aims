import Foundation

public struct ResetBalance: Decodable, Sendable {
    public let available: Int
    public let credits: [ResetCredit]?

    public var detailCount: Int { credits?.filter { $0.status == "available" }.count ?? 0 }
    public var detailsAreIncomplete: Bool { available > detailCount }
}
