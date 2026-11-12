import Foundation

public struct ResetBalance: Decodable, Sendable {
    public let available: Int
    public let credits: [ResetCredit]?
}
