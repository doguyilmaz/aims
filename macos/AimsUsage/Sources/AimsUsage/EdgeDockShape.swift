import SwiftUI

struct EdgeDockShape: Shape {
    var side: EdgeDockSide = .right

    func path(in rect: CGRect) -> Path {
        let edge = rect.maxX
        let curve = rect.width * 0.375
        let radius = rect.width * 0.34375
        let shoulder = rect.width * 0.125
        var path = Path()
        path.move(to: CGPoint(x: edge, y: rect.minY))
        path.addCurve(
            to: CGPoint(x: edge - curve, y: rect.minY + curve),
            control1: CGPoint(x: edge, y: rect.minY + curve),
            control2: CGPoint(x: edge - shoulder, y: rect.minY + curve))
        path.addLine(to: CGPoint(x: rect.minX + radius, y: rect.minY + curve))
        path.addQuadCurve(
            to: CGPoint(x: rect.minX, y: rect.minY + curve + radius),
            control: CGPoint(x: rect.minX, y: rect.minY + curve))
        path.addLine(to: CGPoint(x: rect.minX, y: rect.maxY - curve - radius))
        path.addQuadCurve(
            to: CGPoint(x: rect.minX + radius, y: rect.maxY - curve),
            control: CGPoint(x: rect.minX, y: rect.maxY - curve))
        path.addLine(to: CGPoint(x: edge - curve, y: rect.maxY - curve))
        path.addCurve(
            to: CGPoint(x: edge, y: rect.maxY),
            control1: CGPoint(x: edge - shoulder, y: rect.maxY - curve),
            control2: CGPoint(x: edge, y: rect.maxY - curve))
        path.closeSubpath()
        return side == .right
            ? path
            : path.applying(CGAffineTransform(a: -1, b: 0, c: 0, d: 1, tx: rect.minX + rect.maxX, ty: 0))
    }
}
