// #338 collision classes: prototype + definition, #if twins, redefined macro.
static int helper(int x);

class Console {
public:
    void move_cursor(int x);
};

#if defined(_WIN32)
#define PATH_SEP '\\'
void Console::move_cursor(int x) { (void)x; }
#else
#define PATH_SEP '/'
void Console::move_cursor(int x) { (void)x; }
#endif

static int helper(int x) { return x + 1; }
