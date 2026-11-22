package fixture;

// Each pair below is two distinct declarations that today share one entity ID.
public class Overloads {
    // Constructor overloads.
    public Overloads() {}
    public Overloads(String name) {}

    // Method overloads that differ only in parameter types.
    void visit(String text) {}
    void visit(Integer count) {}

    // Array dimensions are part of the signature.
    void m(int value) {}
    void m(int[] values) {}

    // Same name as the class: a method, not a constructor.
    void Overloads(int legacy) {}
}
