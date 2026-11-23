/* #338 collision classes: prototype + definition, #if twins, redefined macro. */
struct node;
static int helper(int x);

#ifdef _WIN32
#define SEP '\\'
static void move_cursor(int x) { (void)x; }
#else
#define SEP '/'
static void move_cursor(int x) { (void)x; }
#endif

struct node { int value; };

static int helper(int x) { return x + 1; }

int use(void) { move_cursor(1); return helper(SEP); }
