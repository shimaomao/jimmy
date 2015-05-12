package redis_test

import (
	"fmt"

	netURL "net/url"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/timehop/jimmy/redis"
)

var _ = Describe("Connection", func() {
	Describe("NewConnection", func() {
		// Assumes redis' default state is auth-less

		Context("server has no auth set", func() {
			It("should ping without auth", func() {
				url := "redis://localhost:6379"
				parsedURL, _ := netURL.Parse(url)
				c, err := redis.NewConnection(parsedURL)
				Expect(err).To(BeNil())

				_, err = c.Do("PING")
				Expect(err).To(BeNil())
			})
			It("should fallback to ping without auth", func() {
				url := "redis://user:testpass@localhost:6379"
				parsedURL, _ := netURL.Parse(url)
				c, err := redis.NewConnection(parsedURL)
				Expect(err).To(BeNil())

				_, err = c.Do("PING")
				Expect(err).To(BeNil())
			})
		})

		Context("server requires auth", func() {
			BeforeEach(func() {
				url := "redis://localhost:6379"
				parsedURL, _ := netURL.Parse(url)
				c, _ := redis.NewConnection(parsedURL)
				c.Do("CONFIG", "SET", "requirepass", "testpass")
			})
			AfterEach(func() {
				url := "redis://:testpass@localhost:6379"
				parsedURL, _ := netURL.Parse(url)
				c, _ := redis.NewConnection(parsedURL)
				c.Do("CONFIG", "SET", "requirepass", "")
			})

			It("should fail to ping without auth", func() {
				url := "redis://localhost:6379"
				parsedURL, _ := netURL.Parse(url)
				c, _ := redis.NewConnection(parsedURL)

				_, err := c.Do("PING")
				Expect(err).ToNot(BeNil())
			})
			It("should successfully ping with auth", func() {
				url := "redis://:testpass@localhost:6379"
				parsedURL, _ := netURL.Parse(url)
				c, _ := redis.NewConnection(parsedURL)

				_, err := c.Do("PING")
				Expect(err).To(BeNil())
			})
		})
	})

	// Using an arbitrary password should fallback to using no password

	url := "redis://:foopass@localhost:6379"
	parsedURL, _ := netURL.Parse(url)
	c, _ := redis.NewConnection(parsedURL)
	Describe("PFAdd", func() {
		It("Should indicate HyperLogLog register was altered (ie: 1)", func() {
			// Clean up the key
			c.Del("_tests:jimmy:redis:foo1")

			// Subject
			i, err := c.PFAdd("_tests:jimmy:redis:foo1", "bar")
			Expect(err).To(BeNil())
			Expect(i).To(Equal(1))
		})
		It("Should indicate HyperLogLog register was not altered (ie: 0)", func() {

			// Subject
			_, err := c.PFAdd("_tests:jimmy:redis:foo2", "bar")
			Expect(err).To(BeNil())
			i, err := c.PFAdd("_tests:jimmy:redis:foo2", "bar")
			Expect(err).To(BeNil())
			Expect(i).To(Equal(0))
		})
	})

	Describe("PFCount", func() {
		It("Should return the approximate cardinality of the HLL", func() {
			c.Del("_tests:jimmy:redis:foo3")
			var actualCardinality float64 = 20000
			for i := 0; float64(i) < actualCardinality; i++ {
				_, err := c.PFAdd("_tests:jimmy:redis:foo3", fmt.Sprint(i))
				Expect(err).To(BeNil())
			}
			card, err := c.PFCount("_tests:jimmy:redis:foo3")
			Expect(err).To(BeNil())
			// Check a VERY rough 20% accuracy
			Expect(float64(card)).To(BeNumerically("<", actualCardinality*1.2))
			Expect(float64(card)).To(BeNumerically(">", actualCardinality*(1-0.2)))
		})
	})

	Describe("PFMerge", func() {
		It("Should return the approximate cardinality of the union of multiple HLLs", func() {
			c.Del("_tests:jimmy:redis:hll1")
			c.Del("_tests:jimmy:redis:hll2")
			c.Del("_tests:jimmy:redis:hll3")

			setA := []int{1, 2, 3, 4, 5}
			setB := []int{3, 4, 5, 6, 7}
			setC := []int{8, 9, 10, 11, 12}

			for _, x := range setA {
				_, err := c.PFAdd("_tests:jimmy:redis:hll1", fmt.Sprint(x))
				Expect(err).To(BeNil())
			}

			for _, x := range setB {
				_, err := c.PFAdd("_tests:jimmy:redis:hll2", fmt.Sprint(x))
				Expect(err).To(BeNil())
			}

			for _, x := range setC {
				_, err := c.PFAdd("_tests:jimmy:redis:hll3", fmt.Sprint(x))
				Expect(err).To(BeNil())
			}

			for i := 1; i < 4; i++ {
				card, err := c.PFCount(fmt.Sprintf("_tests:jimmy:redis:hll%d", i))
				Expect(err).To(BeNil())
				Expect(card).To(Equal(5))
			}

			ok, err := c.PFMerge("_tests:jimmy:redis:hll1+2", "_tests:jimmy:redis:hll1", "_tests:jimmy:redis:hll2")
			Expect(err).To(BeNil())
			Expect(ok).To(BeTrue())

			card, err := c.PFCount("_tests:jimmy:redis:hll1+2")
			Expect(err).To(BeNil())
			Expect(card).To(Equal(7))

			ok, err = c.PFMerge("_tests:jimmy:redis:hll1+3", "_tests:jimmy:redis:hll1", "_tests:jimmy:redis:hll3")
			Expect(err).To(BeNil())
			Expect(ok).To(BeTrue())

			card, err = c.PFCount("_tests:jimmy:redis:hll1+3")
			Expect(err).To(BeNil())
			Expect(card).To(Equal(10))

			ok, err = c.PFMerge("_tests:jimmy:redis:hll1+2+3", "_tests:jimmy:redis:hll1", "_tests:jimmy:redis:hll2", "_tests:jimmy:redis:hll3")
			Expect(err).To(BeNil())
			Expect(ok).To(BeTrue())

			card, err = c.PFCount("_tests:jimmy:redis:hll1+2+3")
			Expect(err).To(BeNil())
			Expect(card).To(Equal(12))
		})
	})

	Describe("LTrim", func() {
		Context("When a list is trimmed", func() {
			It("Trims the list", func() {
				key := "_tests:jimmy:redis:list"

				c.Del(key)
				for i := 0; i < 5; i++ {
					c.LPush(key, fmt.Sprint(i))
				}

				size, err := c.LLen(key)
				Expect(err).To(BeNil())
				Expect(size).To(Equal(5))

				// Trim nothing
				err = c.LTrim(key, 0, 4)
				Expect(err).To(BeNil())

				size, err = c.LLen(key)
				Expect(err).To(BeNil())
				Expect(size).To(Equal(5))

				// Trim first element
				err = c.LTrim(key, 1, 5)
				Expect(err).To(BeNil())

				size, err = c.LLen(key)
				Expect(err).To(BeNil())
				Expect(size).To(Equal(4))

				item, err := c.LPop(key)
				Expect(err).To(BeNil())
				Expect(item).To(Equal("3"))

				// Trim last element
				err = c.LTrim(key, -4, -1)
				Expect(err).To(BeNil())

				size, err = c.LLen(key)
				Expect(err).To(BeNil())
				Expect(size).To(Equal(3))

				item, err = c.LPop(key)
				Expect(err).To(BeNil())
				Expect(item).To(Equal("2"))
			})
		})

		Context("When a not-list is trimmed", func() {
			It("Returns an error", func() {
				key := "_tests:jimmy:redis:not-list"

				c.Del(key)
				Expect(c.Set(key, "yay")).To(BeNil())
				Expect(c.LTrim(key, 0, 4)).ToNot(BeNil())

				c.Del(key)
				_, err := c.SAdd(key, "yay")
				Expect(err).To(BeNil())
				Expect(c.LTrim(key, 0, 4)).ToNot(BeNil())
			})
		})
	})

	Describe("LRange", func() {
		Context("When an empty list is ranged", func() {
			It("Returns nothing, but no err", func() {
				key := "_tests:jimmy:redis:list"
				c.Del(key)

				things, err := c.LRange(key, 0, -1)
				Expect(err).To(BeNil())
				Expect(things).To(BeEmpty())
			})
		})

		Context("When a list is ranged", func() {
			It("Returns the items", func() {
				key := "_tests:jimmy:redis:list"
				c.Del(key)

				for i := 0; i < 5; i++ {
					_, err := c.LPush(key, fmt.Sprint(i))
					Expect(err).To(BeNil())
				}

				things, err := c.LRange(key, 0, -1)
				Expect(err).To(BeNil())
				Expect(len(things)).To(Equal(5))

				things, err = c.LRange(key, 0, 0)
				Expect(err).To(BeNil())
				Expect(len(things)).To(Equal(1))
				Expect(things[0]).To(Equal("4"))

				things, err = c.LRange(key, -1, -1)
				Expect(err).To(BeNil())
				Expect(len(things)).To(Equal(1))
				Expect(things[0]).To(Equal("0"))
			})
		})
	})

	Describe("SMove", func() {
		It("Should move the member to the other set", func() {
			key := "_tests:jimmy:redis:smove"
			c.Del(key + ":a")
			c.Del(key + ":b")

			c.SAdd(key+":a", "foobar")

			moved, err := c.SMove(key+":a", key+":b", "foobar")
			Expect(err).To(BeNil())
			Expect(moved).To(BeTrue())

			members, _ := c.SMembers(key + ":a")
			Expect(len(members)).To(Equal(0))

			members, _ = c.SMembers(key + ":b")
			Expect(members).To(Equal([]string{"foobar"}))
		})
	})

	Describe("SScan", func() {
		It("Should scan the set", func() {
			key := "_tests:jimmy:redis:sscan"
			c.Del(key)

			c.SAdd(key, "a", "b", "c", "d", "e")

			var scanned []string
			var cursor int
			var matches []string
			var err error

			cursor, matches, err = c.SScan(key, cursor, "", 1)
			Expect(err).To(BeNil())
			scanned = append(scanned, matches...)
			for cursor != 0 {
				cursor, matches, err = c.SScan(key, cursor, "", 1)
				Expect(err).To(BeNil())
				scanned = append(scanned, matches...)
			}

			Expect(len(scanned)).To(Equal(5))
			Expect(scanned).To(ContainElement("a"))
			Expect(scanned).To(ContainElement("b"))
			Expect(scanned).To(ContainElement("c"))
			Expect(scanned).To(ContainElement("d"))
			Expect(scanned).To(ContainElement("e"))
		})
	})
})
